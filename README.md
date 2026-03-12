# KEDA GPU Scaler for AI & HPC Workloads

[![CI](https://github.com/pmady/keda-gpu-scaler/actions/workflows/ci.yaml/badge.svg)](https://github.com/pmady/keda-gpu-scaler/actions/workflows/ci.yaml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
![KEDA: v2.10+](https://img.shields.io/badge/KEDA-v2.10%2B-orange)
![Kubernetes: v1.24+](https://img.shields.io/badge/Kubernetes-v1.24%2B-blue)
![Go: 1.25+](https://img.shields.io/badge/Go-1.25%2B-00ADD8)

An external scaler for [KEDA (Kubernetes Event-driven Autoscaling)](https://keda.sh/) designed specifically to autoscale AI inference, Large Language Model (LLM) deployments, and High-Performance Computing (HPC) workloads based on **native GPU hardware telemetry**.

---

## The Problem: Why a Custom GPU Scaler?

Standard Kubernetes autoscaling (HPA) and traditional KEDA scalers rely on CPU and system memory metrics. However, deep learning and AI inference workloads are almost exclusively bottlenecked by **GPU Compute (SM Utilization)** and **GPU VRAM Allocation**.

If an LLM inference pod experiences a spike in requests, its CPU usage might remain flat while the GPU queue overflows, leading to severe latency or dropped requests.

Today, the workaround requires deploying DCGM Exporter as a sidecar, scraping it into Prometheus, and writing custom PromQL in KEDA's Prometheus scaler. **keda-gpu-scaler eliminates that entire stack** — it reads GPU metrics directly from NVML on each node and serves them to KEDA over gRPC.

### Why Not a Native KEDA Scaler?

Embedding GPU support directly inside KEDA core is architecturally impossible for two reasons:

1. **CGO Constraint**: NVIDIA's Go bindings ([`go-nvml`](https://github.com/NVIDIA/go-nvml)) require CGO. KEDA builds with `CGO_ENABLED=0`.
2. **Node-Level Hardware Access**: The KEDA operator runs as a central pod. NVML requires local GPU device access via `libnvidia-ml.so`, which only a **DaemonSet on GPU nodes** can provide.
3. **Independent Release Cycle**: Ship GPU scaling improvements without waiting for KEDA release cycles.

This scaler bridges that gap by running as a lightweight DaemonSet on every GPU node, collecting hardware metrics natively, and exposing them to KEDA via the [External Scaler gRPC protocol](https://keda.sh/docs/latest/concepts/external-scalers/).

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

**Data Flow:**
1. **Telemetry Ingestion** — The DaemonSet pod calls NVML directly on the local GPU hardware to collect real-time metrics (utilization, VRAM, temperature, power).
2. **KEDA gRPC Interface** — Exposes `IsActive`, `StreamIsActive`, `GetMetricSpec`, and `GetMetrics` endpoints per the KEDA External Scaler contract.
3. **ScaledObject Trigger** — Kubernetes deployments scale up/down (including to zero) based on GPU thresholds defined in the ScaledObject.

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

### 1. Deploy the External Scaler

```bash
helm install keda-gpu-scaler deploy/helm/keda-gpu-scaler \
  --namespace keda \
  --set nodeSelector."nvidia\.com/gpu\.present"=true
```

This deploys a DaemonSet that runs on every GPU node in your cluster.

### 2. Create a ScaledObject

Point your deployment at the scaler using a profile:

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: vllm-gpu-scaler
  namespace: default
spec:
  scaleTargetRef:
    name: vllm-deployment          # Your LLM inference Deployment
  minReplicaCount: 0                # Scale to zero when GPU is idle
  maxReplicaCount: 8
  cooldownPeriod: 60
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

## Building from Source

```bash
# Build binary (requires CGO for NVML)
make build

# Run unit tests
make test

# Run linter
make lint

# Generate protobuf Go code
make proto

# Build Docker image
make docker-build

# Lint Helm chart
make helm-lint
```

---

## Contributing

Contributions are welcome! If you are running into issues with specific GPU architectures or want to add support for AMD ROCm telemetry, please open an issue or submit a pull request. See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0. See [LICENSE](LICENSE) for details.
