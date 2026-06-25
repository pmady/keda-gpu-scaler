# KEDA GPU Scaler

**Scale Kubernetes GPU workloads from real hardware metrics. No DCGM. No PromQL. Optional Prometheus metrics built in.**

[![CI](https://github.com/pmady/keda-gpu-scaler/actions/workflows/ci.yaml/badge.svg)](https://github.com/pmady/keda-gpu-scaler/actions/workflows/ci.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/pmady/keda-gpu-scaler)](https://goreportcard.com/report/github.com/pmady/keda-gpu-scaler)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

A [KEDA External Scaler](https://keda.sh/docs/latest/concepts/external-scalers/) that reads NVIDIA GPU metrics directly from NVML C-bindings and autoscales your vLLM, Triton, and custom inference deployments — including scale-to-zero.

## Why This Exists

Kubernetes HPA watches CPU and memory. It can't see GPU utilization. Your vLLM pod shows 8% CPU while the GPU is at 100%.

The usual fix is dcgm-exporter → Prometheus → KEDA, but that's 5 components and 15-30s of latency.

This project reads GPU metrics directly from NVML and serves them to KEDA over gRPC. 2 components, 2-4 second latency.

### Why Not a Native KEDA Scaler?

Putting GPU support inside KEDA core doesn't work:

1. **CGO Constraint**: NVIDIA's Go bindings ([`go-nvml`](https://github.com/NVIDIA/go-nvml)) require `CGO_ENABLED=1`. KEDA builds with `CGO_ENABLED=0`.
2. **Node-Level Hardware Access**: The KEDA operator runs as a central pod. NVML requires local GPU device access via `libnvidia-ml.so`, which only a **DaemonSet on GPU nodes** can provide.
3. **Independent Release Cycle**: Ship GPU scaling improvements without waiting for KEDA release cycles.

This design is documented in [KEDA issue #7538](https://github.com/kedacore/keda/issues/7538).

---

## Architecture

<p align="center">
  <img src="docs/images/architecture.png" alt="keda-gpu-scaler architecture" width="100%"/>
</p>

1. **DaemonSet** — Runs on nodes labeled with `nvidia.com/gpu.present: "true"`.
2. **NVML Bindings** — Directly reads Streaming Multiprocessor (SM) utilization and Frame Buffer Memory via `go-nvml` C-bindings.
3. **gRPC Interface** — Implements `externalscaler.ExternalScalerServer` (`IsActive`, `StreamIsActive`, `GetMetricSpec`, `GetMetrics`) to natively integrate with the central KEDA operator.
4. **ScaledObject Trigger** — Kubernetes deployments scale up/down (including to zero) based on GPU thresholds defined in the ScaledObject.

---

## GPU Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `gpu_utilization` | GPU compute (SM) utilization | % (0-100) |
| `memory_utilization` | GPU memory controller utilization | % (0-100) |
| `memory_used_mib` | GPU VRAM used | MiB |
| `memory_used_percent` | GPU VRAM used as percentage of total | % (0-100) |
| `temperature` | GPU die temperature | Celsius |
| `power_draw` | GPU power consumption | Watts |
| `pcie_tx_kbps` | PCIe transmit throughput (CPU→GPU) | KB/s |
| `pcie_rx_kbps` | PCIe receive throughput (GPU→CPU) | KB/s |
| `nvlink_tx_mbps` | NVLink transmit throughput (GPU→GPU) | MB/s |
| `nvlink_rx_mbps` | NVLink receive throughput (GPU→GPU) | MB/s |

---

## Pre-built Scaling Profiles

Instead of configuring raw metric thresholds, use a profile optimized for your workload:

| Profile | Primary Metric | Target | Activation | Use Case |
|---------|---------------|--------|------------|----------|
| `vllm-inference` | Memory % | 80 | 5 | vLLM / LLM serving with scale-to-zero |
| `triton-inference` | GPU Util | 75 | 10 | NVIDIA Triton Inference Server |
| `training` | GPU Util | 90 | 0 | Training jobs (no scale-to-zero) |
| `batch` | Memory % | 70 | 1 | Batch inference with aggressive scale-down |
| `distributed-training` | NVLink TX | 800 | 100 | Data-parallel training on NVLink systems |

---

## Prerequisites

- A Kubernetes cluster (e.g., **OKE**, GKE, EKS, AKS) with **NVIDIA GPU worker nodes**
- [KEDA v2.10+](https://keda.sh/docs/latest/deploy/) installed in the cluster
- NVIDIA GPU drivers and [Device Plugin](https://github.com/NVIDIA/k8s-device-plugin) installed

---

## Quick Start

### 1. Deploy the Scaler

Deploy the DaemonSet and gRPC service into your cluster. (Ensure KEDA is already installed.)

```bash
kubectl apply -f deploy/manifests.yaml
```

This deploys a DaemonSet that runs on every GPU node in your cluster, plus a ClusterIP Service for KEDA to discover it.

Or use Helm:

```bash
helm install keda-gpu-scaler deploy/helm/keda-gpu-scaler \
  --namespace keda \
  --set nodeSelector."nvidia\.com/gpu\.present"=true
```

### 2. Attach to your AI Workload

Create a ScaledObject pointing to the external scaler service:

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: vllm-inference-scaler
  namespace: ai-workloads
spec:
  scaleTargetRef:
    name: vllm-deepseek-deployment
  minReplicaCount: 1
  maxReplicaCount: 50
  triggers:
    - type: external
      metadata:
        scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
        targetGpuUtilization: "80"
```

Or use a pre-built profile:

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "vllm-inference"
```

### 3. Custom Configuration

Override any profile default or use raw GPU metrics directly:

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "gpu_utilization"
      targetValue: "85"
      activationThreshold: "10"
      gpuIndex: "0"              # specific GPU index, or omit for all
      aggregation: "max"         # max, min, avg, sum across GPUs
```

See `deploy/examples/` for ready-to-use ScaledObject manifests.

---

## Configuration Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `profile` | Pre-built scaling profile name | (none) |
| `metricType` | GPU metric to scale on | `gpu_utilization` |
| `targetValue` | Target metric value for scaling | `80` |
| `targetGpuUtilization` | Shorthand for GPU utilization target | (none) |
| `targetMemoryUtilization` | Shorthand for VRAM utilization target | (none) |
| `activationThreshold` | Value below which scale-to-zero activates | `0` |
| `gpuIndex` | Specific GPU index to monitor | `-1` (all GPUs) |
| `aggregation` | Multi-GPU aggregation: `max`, `min`, `avg`, `sum` | `max` |
| `pollIntervalSeconds` | Metric polling interval | `10` |

---

## Prometheus Metrics (Optional)

The scaler exposes an optional Prometheus-compatible `/metrics` endpoint for monitoring the scaler itself and GPU fleet health. **This is independent of the KEDA scaling path** — scaling works identically with or without it.

### Enable/Disable

```bash
# Enabled by default on port 9090
--metrics-port=9090

# Disable entirely (zero overhead)
--metrics-port=0
```

Helm:
```yaml
metrics:
  enabled: true   # set to false to disable
  port: 9090
```

### Exposed Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `keda_gpu_scaler_gpu_utilization_percent` | Gauge | GPU compute utilization (per GPU) |
| `keda_gpu_scaler_gpu_memory_used_bytes` | Gauge | GPU memory in use (per GPU) |
| `keda_gpu_scaler_gpu_memory_total_bytes` | Gauge | Total GPU memory (per GPU) |
| `keda_gpu_scaler_gpu_temperature_celsius` | Gauge | GPU temperature (per GPU) |
| `keda_gpu_scaler_gpu_power_draw_watts` | Gauge | GPU power draw (per GPU) |
| `keda_gpu_scaler_collections_total` | Counter | Total NVML collection calls |
| `keda_gpu_scaler_collection_errors_total` | Counter | Failed NVML collection calls |
| `keda_gpu_scaler_collection_duration_seconds` | Histogram | NVML collection latency |
| `keda_gpu_scaler_scaler_requests_total` | Counter | gRPC requests by method |
| `keda_gpu_scaler_scaler_request_errors_total` | Counter | gRPC errors by method |

All per-GPU metrics are labeled with `gpu_index`, `gpu_uuid`, and `gpu_name`.

## Kubernetes Probes

The scaler exposes liveness and readiness endpoints on a dedicated probe port:

- `/healthz` returns `200` while the process is alive.
- `/readyz` returns `200` after NVML initializes and the first metrics collection succeeds.

```bash
--probe-port=8081
```

Helm:
```yaml
probes:
  enabled: true
  port: 8081
```

---

## Build it Yourself

This project requires `CGO_ENABLED=1` to compile the NVIDIA C-bindings.

> [!NOTE]
> The compiled binaries (`keda-gpu-scaler` and `gpu-metrics`) dynamically link NVIDIA's NVML library and load `libnvidia-ml.so` at runtime. They will **fail to start on any machine that does not have the NVIDIA driver installed** (which provides `libnvidia-ml.so`) — for example, a laptop or CI runner with no NVIDIA GPU. You can still build, lint, and run the test suite without a GPU, since the tests use a mock collector (see [Can I run this without a GPU?](docs/FAQ.md#can-i-run-this-without-a-gpu-for-development)).

```bash
# Build KEDA scaler binary (requires CGO for NVML)
make build

# Build standalone GPU metrics CLI (no KEDA/gRPC needed)
make build-metrics

# Build all binaries
make build-all

# Run unit tests
make test

# Run linter
make lint

# Generate protobuf Go code
make proto

# Build and push a release image
make docker-release VERSION=v0.1.0

# Deploy to cluster
make deploy
```

### Checking the Version

Both binaries accept a `--version` flag (and a bare `version` argument) that
prints the version, Go version, and build date, then exits. Unlike normal
operation, this does **not** require a GPU or the NVIDIA driver:

```bash
keda-gpu-scaler --version    # keda-gpu-scaler v0.5.0 (go1.26.4, built 2026-06-25)
gpu-metrics --version        # gpu-metrics v0.5.0 (go1.26.4, built 2026-06-25)
```

`make build` stamps the version from `git describe` at link time; builds without
ldflags (e.g. `go run`) report `dev`.

### Standalone GPU Metrics CLI

Collect GPU metrics without Kubernetes — works on bare metal, SLURM jobs, Flux jobs, Kubernetes pods, and Singularity containers. The same binary and the same JSON schema work everywhere.

> [!IMPORTANT]
> `gpu-metrics` requires `libnvidia-ml.so` (installed with the NVIDIA driver) on the host. On a machine without an NVIDIA driver it exits immediately with `nvml init failed`.

```bash
gpu-metrics                       # one-shot table output (env auto-detected)
gpu-metrics --format json         # JSON for scripting
gpu-metrics --format csv          # CSV for analysis
gpu-metrics --interval 5s         # continuous collection
gpu-metrics --device 0 --quiet    # single GPU, no logs
gpu-metrics --env slurm           # force environment (auto|k8s|slurm|flux|standalone)
gpu-metrics --version             # print version and exit (no GPU/NVML required)
```

The `--env` flag auto-detects the orchestrator by default. Detection priority: **SLURM → Flux → Kubernetes → standalone**.

Every environment emits the same unified JSON schema with an `environment` block so you can compare GPU performance across on-prem and cloud with identical tooling:

```json
{
  "environment": { "orchestrator": "slurm", "node": "compute-01", "job_id": "123", "task_rank": 0 },
  "driver_version": "535.104.05",
  "collected_at": "2026-06-17T10:00:00Z",
  "devices": [...]
}
```

**SLURM** — auto-detected when `SLURM_JOB_ID` is set; collects only the GPUs assigned to your job step:

```bash
srun --gres=gpu:2 gpu-metrics --format json
```

**Flux** — auto-detected when `FLUX_JOB_ID` is set; collects only the GPUs in `CUDA_VISIBLE_DEVICES`:

```bash
flux run -N1 -g2 gpu-metrics --format json
```

See **[HPC & Cross-Environment Metrics](docs/hpc.md)** for full usage, and **[Cross-Environment Comparison Guide](docs/cross-env-comparison.md)** for comparing on-prem vs cloud GPU runs.

Or build the Docker image directly:

```bash
docker build -t your-registry/keda-gpu-scaler:v0.1.0 .
docker push your-registry/keda-gpu-scaler:v0.1.0
```

---

## How It Compares

| | keda-gpu-scaler | dcgm-exporter + Prometheus | Custom Metrics API |
|---|---|---|---|
| **Components** | 1 DaemonSet (+ optional /metrics) | dcgm-exporter + Prometheus + adapter | Custom metrics server |
| **Metric latency** | Sub-second (direct NVML) | 15-30s (scrape interval) | Depends on implementation |
| **Scale-to-zero** | Yes (KEDA native) | Yes (with KEDA Prometheus scaler) | Manual |
| **Configuration** | 3-line ScaledObject | PromQL query per metric | Custom code |
| **GPU metrics** | 10 hardware metrics | 50+ DCGM metrics | Whatever you build |
| **Dependencies** | KEDA, NVIDIA drivers | KEDA, Prometheus, dcgm-exporter | Varies |
| **Failure domain** | Node-local | Centralized Prometheus | Varies |

---

## Documentation

- **[Design Document](docs/DESIGN.md)** — Architecture decisions, gRPC interface, scaling profiles, testing strategy
- **[Migration Guide](docs/MIGRATION.md)** — Replace dcgm-exporter + Prometheus with keda-gpu-scaler
- **[HPC & Cross-Environment Metrics](docs/hpc.md)** — SLURM, Flux, Kubernetes, and standalone GPU metrics
- **[Cross-Environment Comparison](docs/cross-env-comparison.md)** — Compare GPU performance across on-prem and cloud
- **[FAQ](docs/FAQ.md)** — Common questions about GPU scaling, MIG, multi-GPU, scale-to-zero
- **[Changelog](CHANGELOG.md)** — Release history

---

## Related

- [CNCF Blog: GPU Autoscaling on Kubernetes with KEDA](https://www.cncf.io/blog/2026/05/27/gpu-autoscaling-on-kubernetes-with-keda-building-an-external-scaler/)
- [KEDA issue #7538](https://github.com/kedacore/keda/issues/7538) — original discussion
- [CNCF TOC initiative #2188](https://github.com/cncf/toc/issues/2188) — whitepaper proposal

---

## Adopters

Using keda-gpu-scaler? Add your organization to [ADOPTERS.md](ADOPTERS.md).

---

## Roadmap

- AMD ROCm support
- MIG per-instance metrics
- vLLM queue depth scaling

---

## Contributors

Thanks to everyone who helps build keda-gpu-scaler.

<!-- readme: contributors -start -->
<table>
<tr>
    <td align="center"><a href="https://github.com/pmady"><img src="https://avatars.githubusercontent.com/u/15876315?v=4" width="80;" alt="pmady"/><br /><sub><b>Pavan Madduri</b></sub></a></td>
    <td align="center"><a href="https://github.com/venkata22a"><img src="https://avatars.githubusercontent.com/u/31258325?v=4" width="80;" alt="venkata22a"/><br /><sub><b>venkata22a</b></sub></a></td>
    <td align="center"><a href="https://github.com/pen-pal"><img src="https://avatars.githubusercontent.com/u/61139563?v=4" width="80;" alt="pen-pal"/><br /><sub><b>Manish Khadka</b></sub></a></td>
    <td align="center"><a href="https://github.com/ibobgunardi"><img src="https://avatars.githubusercontent.com/u/24878946?v=4" width="80;" alt="ibobgunardi"/><br /><sub><b>Bobi Gunardi</b></sub></a></td>
    <td align="center"><a href="https://github.com/KaustAbhinand"><img src="https://avatars.githubusercontent.com/u/154255646?v=4" width="80;" alt="KaustAbhinand"/><br /><sub><b>Kaustubh Abhinand</b></sub></a></td>
    <td align="center"><a href="https://github.com/Atharv-AC"><img src="https://avatars.githubusercontent.com/u/235652593?v=4" width="80;" alt="Atharv-AC"/><br /><sub><b>Atharv</b></sub></a></td>
</tr>
</table>
<!-- readme: contributors -end -->

See [CONTRIBUTORS.md](CONTRIBUTORS.md) for detailed contributions.

## Contributing

Contributions welcome — GPU autoscaling use cases, vendor support (AMD ROCm, Intel), or docs improvements. See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
