# HPC Workload Manager Integration

The standalone `gpu-metrics` CLI collects GPU metrics via NVML without requiring Kubernetes or KEDA. It auto-detects common HPC schedulers and scopes metrics to the GPUs allocated to your job.

> [!NOTE]
> `gpu-metrics` requires `libnvidia-ml.so` (installed with the NVIDIA driver) on the host. It exits immediately with `nvml init failed` on machines without an NVIDIA driver.

---

## SLURM

[SLURM](https://slurm.schedmd.com/) is the dominant workload manager in academic and government HPC clusters. When you launch `gpu-metrics` inside a SLURM job, it automatically reads `SLURM_JOB_ID` and the assigned GPU indices from `SLURM_STEP_GPUS` (falling back to `SLURM_JOB_GPUS`, `GPU_DEVICE_ORDINAL`, then `CUDA_VISIBLE_DEVICES`).

### Detection

SLURM mode activates automatically (`--slurm auto`, the default) when `SLURM_JOB_ID` is set. You can force it on or off:

```bash
gpu-metrics --slurm auto   # default: activate if inside a SLURM job
gpu-metrics --slurm on     # always treat as SLURM job
gpu-metrics --slurm off    # ignore SLURM environment
```

### Usage

```bash
# One-shot table output — only shows GPUs allocated to this job
srun --gres=gpu:2 gpu-metrics

# JSON with SLURM job context
srun --gres=gpu:2 gpu-metrics --format json

# Continuous collection every 5 seconds
srun --gres=gpu:2 gpu-metrics --interval 5s --format csv

# From a batch script
#SBATCH --gres=gpu:4
gpu-metrics --format json > gpu-metrics-$SLURM_JOB_ID.json
```

### JSON output

When SLURM is detected, a `slurm` block is included in the JSON output:

```json
{
  "slurm": {
    "JobID": "98765",
    "JobName": "train-llm",
    "Partition": "gpu-a100",
    "NodeName": "node02",
    "ProcID": 8,
    "LocalID": 2,
    "GPUs": "0,1,2,3"
  },
  "devices": [...]
}
```

### GPU assignment

SLURM exposes assigned GPUs via these env vars, checked in priority order:

| Variable | Description |
|----------|-------------|
| `SLURM_STEP_GPUS` | GPUs for the current step (most specific) |
| `SLURM_JOB_GPUS` | GPUs for the whole job |
| `GPU_DEVICE_ORDINAL` | Alternative GPU ordinal variable |
| `CUDA_VISIBLE_DEVICES` | CUDA-level restriction (fallback) |

---

## Flux

[Flux](https://flux-framework.org/) is a next-generation workload manager developed at Lawrence Livermore National Laboratory (LLNL). It is gaining adoption at DOE national labs and is designed for heterogeneous hardware including GPUs. When you launch `gpu-metrics` inside a Flux job, it reads `FLUX_JOB_ID` and the assigned GPUs from `CUDA_VISIBLE_DEVICES`, which Flux sets automatically when GPU affinity is active (the default for jobs submitted with `-g N`).

### Detection

Flux mode activates automatically (`--flux auto`, the default) when `FLUX_JOB_ID` is set. You can force it on or off:

```bash
gpu-metrics --flux auto   # default: activate if inside a Flux job
gpu-metrics --flux on     # always treat as Flux job
gpu-metrics --flux off    # ignore Flux environment
```

### Usage

```bash
# One-shot table output — only shows GPUs allocated to this task
flux run -N1 -g1 gpu-metrics

# JSON with Flux job context
flux run -N1 -g2 gpu-metrics --format json

# Continuous collection every 5 seconds
flux run -N1 -g4 gpu-metrics --interval 5s --format json

# Multi-node: each task collects its own assigned GPUs
flux run -N4 -g2 --tasks-per-node=1 gpu-metrics --format json
```

### JSON output

When Flux is detected, a `flux` block is included in the JSON output:

```json
{
  "flux": {
    "JobID": "f23r45t",
    "TaskRank": 4,
    "LocalID": 0,
    "NumTasks": 8,
    "NumNodes": 2,
    "GPUs": "0,1"
  },
  "devices": [...]
}
```

### GPU assignment

Flux sets `CUDA_VISIBLE_DEVICES` automatically when a job requests GPUs with `-g N` or `--gpus-per-task=N`. NVML honours this restriction, so `gpu-metrics` already sees only the allocated devices. The `flux` JSON block records the original `CUDA_VISIBLE_DEVICES` value for reference.

> [!IMPORTANT]
> If you submit a Flux job **without** GPU affinity (no `-g` flag), `CUDA_VISIBLE_DEVICES` will not be set and `gpu-metrics` will collect from all GPUs on the node. Always submit with `-g N` for correct per-task isolation.

---

## Scheduler auto-detection

`gpu-metrics` checks schedulers in this order and uses the first one it finds:

1. **SLURM** — if `SLURM_JOB_ID` is set (checked first)
2. **Flux** — if `FLUX_JOB_ID` is set (only if SLURM was not detected)
3. **Bare metal** — collect from all visible GPUs

To disable auto-detection for both: `--slurm off --flux off`

---

## CSV output with scheduler context

When a scheduler is active, the scheduler columns are prepended to each CSV row:

**SLURM:**
```
JobID,JobName,Partition,Node,Rank,LocalRank,GPUs,index,uuid,name,...
98765,train-llm,gpu-a100,node02,8,2,0,0,GPU-aaa,A100,...
```

**Flux:**
```
FluxJobID,TaskRank,LocalRank,GPUs,index,uuid,name,...
f23r45t,4,0,0,0,0,GPU-bbb,H100,...
```

---

## Singularity / Apptainer containers

`gpu-metrics` works inside Singularity/Apptainer containers on SLURM or Flux nodes. The NVIDIA runtime passes `CUDA_VISIBLE_DEVICES` into the container, and scheduler env vars are inherited automatically:

```bash
# SLURM + Singularity
srun --gres=gpu:2 singularity exec --nv gpu-metrics.sif gpu-metrics --format json

# Flux + Singularity
flux run -N1 -g2 singularity exec --nv gpu-metrics.sif gpu-metrics --format json
```
