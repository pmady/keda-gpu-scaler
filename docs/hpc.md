# HPC & Cross-Environment GPU Metrics

The standalone `gpu-metrics` CLI collects GPU metrics via NVML without requiring Kubernetes or KEDA. It uses a single `--env` flag to auto-detect the orchestrator and emits a **unified JSON schema** regardless of environment — so you can compare GPU performance across Kubernetes, SLURM, Flux, and bare metal with no post-processing.

> [!NOTE]
> `gpu-metrics` requires `libnvidia-ml.so` (installed with the NVIDIA driver) on the host. It exits immediately with `nvml init failed` on machines without an NVIDIA driver.

---

## Environment flag

```bash
gpu-metrics --env auto       # default: detect from env vars
gpu-metrics --env slurm      # force SLURM mode
gpu-metrics --env flux       # force Flux mode
gpu-metrics --env k8s        # force Kubernetes mode
gpu-metrics --env standalone # bare metal / no scheduler
```

Detection priority when `auto` is used: **SLURM → Flux → Kubernetes → standalone**.

Detection signals:

| Environment | Signal |
|-------------|--------|
| SLURM | `SLURM_JOB_ID` is set |
| Flux | `FLUX_JOB_ID` is set |
| Kubernetes | `KUBERNETES_SERVICE_HOST` is set |
| Standalone | none of the above |

---

## Unified JSON schema

Every environment emits the same top-level structure. The `environment` block identifies where the sample was collected; the `devices` array is always identical in shape.

```json
{
  "environment": {
    "orchestrator": "<k8s|slurm|flux|standalone>",
    "node": "<node name or hostname>",
    "job_id": "<scheduler job id, if any>",
    "task_rank": 0
  },
  "collected_at": "2026-06-17T10:00:00Z",
  "devices": [
    {
      "Index": 0,
      "UUID": "GPU-aaaa-1111",
      "Name": "NVIDIA H100 SXM5 80GB",
      "GPUUtilization": 85,
      "MemoryUtilization": 70,
      "MemoryUsedMiB": 57344,
      "MemoryTotalMiB": 81920,
      "TemperatureCelsius": 72,
      "PowerDrawWatts": 650,
      "PowerLimitWatts": 700,
      "PCIeTxKBps": 4096,
      "PCIeRxKBps": 2048,
      "NVLinkTxMBps": 120000,
      "NVLinkRxMBps": 118000
    }
  ]
}
```

Environment-specific extra fields are also included in the `environment` block when present:

| Field | SLURM | Flux | K8s | Standalone |
|-------|-------|------|-----|------------|
| `node` | ✓ | | ✓ | hostname |
| `job_id` | ✓ | ✓ | | |
| `task_rank` | ✓ (proc rank) | ✓ | | |
| `partition` | ✓ | | | |
| `flux_uri` | | ✓ | | |
| `pod_name` | | | ✓ | |
| `namespace` | | | ✓ | |

---

## SLURM

[SLURM](https://slurm.schedmd.com/) is the dominant workload manager in academic and government HPC clusters. When `SLURM_JOB_ID` is set, `gpu-metrics` automatically scopes collection to the GPUs assigned to your job step.

### GPU assignment

SLURM exposes assigned GPUs via these env vars, checked in priority order:

| Variable | Description |
|----------|-------------|
| `SLURM_STEP_GPUS` | GPUs for the current step (most specific) |
| `SLURM_JOB_GPUS` | GPUs for the whole job |
| `GPU_DEVICE_ORDINAL` | Alternative GPU ordinal variable |
| `CUDA_VISIBLE_DEVICES` | CUDA-level restriction (fallback) |

### Usage

```bash
# One-shot table — only shows GPUs allocated to this job
srun --gres=gpu:2 gpu-metrics

# JSON with SLURM context
srun --gres=gpu:2 gpu-metrics --format json

# Continuous collection every 5 seconds
srun --gres=gpu:2 gpu-metrics --interval 5s --format csv

# From a batch script
#SBATCH --gres=gpu:4
gpu-metrics --format json > gpu-metrics-$SLURM_JOB_ID.json
```

### JSON output

```json
{
  "environment": {
    "orchestrator": "slurm",
    "node": "node02",
    "job_id": "98765",
    "task_rank": 8,
    "partition": "gpu-a100"
  },
  "collected_at": "2026-06-17T10:00:00Z",
  "devices": [...]
}
```

---

## Flux

[Flux](https://flux-framework.org/) is a next-generation workload manager developed at Lawrence Livermore National Laboratory. When `FLUX_JOB_ID` is set, `gpu-metrics` reads the GPUs from `CUDA_VISIBLE_DEVICES`, which Flux sets automatically when GPU affinity is active.

### One-shot / ad hoc usage

```bash
# One-shot table — only shows GPUs allocated to this task
flux run -N1 -g1 gpu-metrics

# JSON with Flux context
flux run -N1 -g2 gpu-metrics --format json

# Multi-node: each task collects its own assigned GPUs
flux run -N4 -g2 --tasks-per-node=1 gpu-metrics --format json
```

### Automatic collection via Flux shell plugin (recommended)

Running `gpu-metrics --interval ...` as its own side-by-side `flux run` command alongside the real job works, but it's manual: you have to remember to launch it, keep the interval in sync, and clean it up yourself. As of the release containing [flux-framework/flux-core#7723](https://github.com/flux-framework/flux-core/pull/7723), Flux ships a generic builtin `coprocess` shell plugin that starts and stops a helper process automatically for the lifetime of a job. keda-gpu-scaler provides a ready-made drop-in for it: [`deploy/flux/gpu-monitor.lua`](../deploy/flux/gpu-monitor.lua).

Install it once per site (or per user, in a personal shell initrc directory):

```bash
cp deploy/flux/gpu-monitor.lua /etc/flux/shell/lua.d/gpu-monitor.lua
```

Then enable it per job with `-o gpu-monitor`:

```bash
flux run -o gpu-monitor -N2 -g1 ./train.sh
```

`gpu-metrics --interval 5s --format json` now starts on every shell rank the moment the job starts, and is stopped (SIGTERM, escalating to SIGKILL after a configurable timeout if it doesn't exit) the moment the job's tasks complete — no separate `flux run` command, and no risk of the collector outliving or lagging the job it's watching. Output is aggregated to `gpu-metrics-<jobid>.jsonl` by default; override the destination (or any other field) with the usual scalar-or-object shell option convention:

```bash
# Per-node output file instead of one aggregated file
flux run -o gpu-monitor.output=/tmp/gpu-{{node.id}}.jsonl -N2 -g1 ./train.sh
```

See the comments in `gpu-monitor.lua` for the full option schema, and `flux-shell-options(7)` / `flux-shell-initrc(5)` in flux-core for how `coprocess.define()` works.

### JSON output

```json
{
  "environment": {
    "orchestrator": "flux",
    "job_id": "f23r45t",
    "task_rank": 4,
    "flux_uri": "local:///run/flux/local"
  },
  "collected_at": "2026-06-17T10:00:00Z",
  "devices": [...]
}
```

> [!IMPORTANT]
> If you submit a Flux job **without** GPU affinity (no `-g` flag), `CUDA_VISIBLE_DEVICES` will not be set and `gpu-metrics` will collect from all GPUs on the node. Always submit with `-g N` for correct per-task isolation.

---

## Kubernetes

When running inside a pod, `gpu-metrics` detects Kubernetes via `KUBERNETES_SERVICE_HOST` and includes pod/node metadata. Expose this via the Downward API:

```yaml
env:
  - name: NODE_NAME
    valueFrom:
      fieldRef:
        fieldPath: spec.nodeName
  - name: POD_NAME
    valueFrom:
      fieldRef:
        fieldPath: metadata.name
  - name: POD_NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace
```

### JSON output

```json
{
  "environment": {
    "orchestrator": "k8s",
    "node": "gpu-node-42",
    "pod_name": "train-job-0",
    "namespace": "ml-workloads"
  },
  "collected_at": "2026-06-17T10:00:00Z",
  "devices": [...]
}
```

---

## Standalone (bare metal)

When no scheduler is detected, `gpu-metrics` falls back to standalone mode and uses the system hostname as the node name.

```bash
gpu-metrics --format json
```

```json
{
  "environment": {
    "orchestrator": "standalone",
    "node": "dev-workstation"
  },
  "collected_at": "2026-06-17T10:00:00Z",
  "devices": [...]
}
```

---

## CSV output

CSV prepends four environment columns before all GPU columns, so every row is fully self-describing:

```
orchestrator,node,job_id,task_rank,index,uuid,name,gpu_util_pct,mem_util_pct,...
slurm,node02,98765,8,0,GPU-aaaa,A100,...
slurm,node02,98765,8,1,GPU-bbbb,A100,...
```

This format is pandas/DuckDB/spreadsheet-friendly. All environments produce the same column order.

---

## Cross-environment comparison

Because the schema is identical across environments, you can compare runs with standard tools:

```bash
# Collect on-prem (SLURM)
srun gpu-metrics --format json > slurm-run.json

# Collect in cloud (Kubernetes pod)
kubectl exec train-pod-0 -- gpu-metrics --format json > k8s-run.json

# Compare average GPU utilization
jq -s '
  map({
    env: .environment.orchestrator,
    node: .environment.node,
    avg_util: (.devices | map(.GPUUtilization) | add / length)
  })
' slurm-run.json k8s-run.json
```

Output:
```json
[
  { "env": "slurm", "node": "compute-01", "avg_util": 84 },
  { "env": "k8s",   "node": "gpu-node-42", "avg_util": 71 }
]
```

See [Cross-Environment Comparison Guide](cross-env-comparison.md) for more recipes.

---

## Singularity / Apptainer containers

`gpu-metrics` works inside Singularity/Apptainer containers on SLURM or Flux nodes. Scheduler env vars are inherited automatically:

```bash
# SLURM + Singularity
srun --gres=gpu:2 singularity exec --nv gpu-metrics.sif gpu-metrics --format json

# Flux + Singularity
flux run -N1 -g2 singularity exec --nv gpu-metrics.sif gpu-metrics --format json
```
