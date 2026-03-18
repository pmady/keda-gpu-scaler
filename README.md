# KEDA NVML GPU Scaler

[![CI](https://github.com/pmady/keda-gpu-scaler/actions/workflows/ci.yaml/badge.svg)](https://github.com/pmady/keda-gpu-scaler/actions/workflows/ci.yaml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
![KEDA: v2.10+](https://img.shields.io/badge/KEDA-v2.10%2B-orange)
![Kubernetes: v1.24+](https://img.shields.io/badge/Kubernetes-v1.24%2B-blue)
![Go: 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8)

A native, high-performance [External Scaler](https://keda.sh/docs/latest/concepts/external-scalers/) for [KEDA](https://keda.sh/) that scales AI/ML workloads directly using NVIDIA Management Library (NVML) C-bindings.

Bypass Prometheus, eliminate metric scraping latency, and scale your vLLM, Triton, and custom inference pods based on true, real-time GPU hardware utilization.

---

## The Problem with Standard GPU Scaling

Scaling massive AI inference workloads (like DeepSeek or Llama 3) on Kubernetes using standard CPU/Memory HPA is fundamentally broken. GPU nodes often sit at 10% CPU utilization while the physical GPUs are 100% saturated with 200+ pending requests in the vLLM queue.

While you can work around this by deploying `dcgm-exporter` and using the KEDA Prometheus scaler, it introduces significant architecture bloat:

- **PromQL queries are brittle** and framework-dependent.
- **Scraping intervals introduce scaling latency**, often 15-30 seconds late.
- It requires maintaining a **centralized Prometheus server** just to read local node hardware states.

**keda-gpu-scaler eliminates that entire stack** — it reads GPU metrics directly from NVML on each node and serves them to KEDA over gRPC.

### Why Not a Native KEDA Scaler?

Embedding GPU support directly inside KEDA core is architecturally impossible for three reasons:

1. **CGO Constraint**: NVIDIA's Go bindings ([`go-nvml`](https://github.com/NVIDIA/go-nvml)) require `CGO_ENABLED=1`. KEDA builds with `CGO_ENABLED=0`.
2. **Node-Level Hardware Access**: The KEDA operator runs as a central pod. NVML requires local GPU device access via `libnvidia-ml.so`, which only a **DaemonSet on GPU nodes** can provide.
3. **Independent Release Cycle**: Ship GPU scaling improvements without waiting for KEDA release cycles.

This design is documented in [KEDA issue #7538](https://github.com/kedacore/keda/issues/7538).

---

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│  GPU Node (DaemonSet)                                    │
│                                                          │
│   ┌───────────────────┐       ┌────────────────────────┐ │
│   │  keda-gpu-scaler  │◄─────►│ NVIDIA GPU (NVML)      │ │
│   │  gRPC :6000       │       │ libnvidia-ml.so        │ │
│   │                   │       │ A100 / H100 / L40S ... │ │
│   └─────────▲─────────┘       └────────────────────────┘ │
│             │                                            │
└─────────────┼────────────────────────────────────────────┘
              │ gRPC (ExternalScaler protocol)
┌─────────────┼────────────────────────────────────────────┐
│  KEDA       │                                            │
│   ┌─────────▼──────────┐      ┌────────────────────────┐ │
│   │  External Scaler   │─────►│  HPA (scale up/down)   │ │
│   │  trigger            │      │  your-vllm-deployment  │ │
│   └────────────────────┘      └────────────────────────┘ │
└──────────────────────────────────────────────────────────┘
```

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

---

## Pre-built Scaling Profiles

Instead of configuring raw metric thresholds, use a profile optimized for your workload:

| Profile | Primary Metric | Target | Activation | Use Case |
|---------|---------------|--------|------------|----------|
| `vllm-inference` | Memory % | 80 | 5 | vLLM / LLM serving with scale-to-zero |
| `triton-inference` | GPU Util | 75 | 10 | NVIDIA Triton Inference Server |
| `training` | GPU Util | 90 | 0 | Training jobs (no scale-to-zero) |
| `batch` | Memory % | 70 | 1 | Batch inference with aggressive scale-down |

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

## Build it Yourself

This project requires `CGO_ENABLED=1` to compile the NVIDIA C-bindings.

```bash
# Build binary (requires CGO for NVML)
make build

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

Or build the Docker image directly:

```bash
docker build -t your-registry/keda-gpu-scaler:v0.1.0 .
docker push your-registry/keda-gpu-scaler:v0.1.0
```

---

## Contributing

Contributions are welcome! If you are running massive LLM inference deployments and need custom NVML metric profiles (e.g., PCIe bandwidth triggers, temperature thresholds), or want to add support for AMD ROCm telemetry, please open an issue or submit a PR. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
