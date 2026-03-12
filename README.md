# keda-gpu-scaler

A native [KEDA](https://keda.sh) External gRPC Scaler for GPU workloads. Scales Kubernetes deployments based on real-time NVIDIA GPU metrics via direct NVML hardware access — no Prometheus required.

## Why?

Standard CPU/memory-based autoscaling fails for GPU workloads. Teams running LLM inference (vLLM, Triton), training jobs, or batch GPU processing need to scale based on actual GPU utilization, VRAM usage, and compute saturation.

Today, this requires deploying DCGM Exporter as a sidecar, scraping it into Prometheus, and writing custom PromQL in KEDA's Prometheus scaler. **keda-gpu-scaler eliminates that entire stack** — it reads GPU metrics directly from NVML on each node and serves them to KEDA over gRPC.

## Architecture

```
┌─────────────────────────────────────────────────────┐
│ GPU Node                                            │
│                                                     │
│  ┌──────────────────┐    ┌───────────────────────┐  │
│  │ keda-gpu-scaler  │◄──►│ NVIDIA GPU (NVML)     │  │
│  │ (DaemonSet pod)  │    │ libnvidia-ml.so       │  │
│  │ gRPC :6000       │    └───────────────────────┘  │
│  └────────▲─────────┘                               │
│           │ gRPC                                    │
└───────────┼─────────────────────────────────────────┘
            │
┌───────────┼─────────────────────────────────────────┐
│ KEDA      │                                         │
│  ┌────────▼─────────┐    ┌───────────────────────┐  │
│  │ External Scaler  │───►│ HPA (scale up/down)   │  │
│  │ trigger           │    │ your-vllm-deployment  │  │
│  └──────────────────┘    └───────────────────────┘  │
└─────────────────────────────────────────────────────┘
```

## Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `gpu_utilization` | GPU compute utilization | % (0-100) |
| `memory_utilization` | GPU memory controller utilization | % (0-100) |
| `memory_used_mib` | GPU memory used | MiB |
| `memory_used_percent` | GPU memory used as percentage | % (0-100) |
| `temperature` | GPU temperature | Celsius |
| `power_draw` | GPU power consumption | Watts |

## Pre-built Profiles

Instead of configuring raw metrics, use a profile that matches your workload:

| Profile | Metric | Target | Activation | Use Case |
|---------|--------|--------|------------|----------|
| `vllm-inference` | Memory % | 80 | 5 | vLLM serving with scale-to-zero |
| `triton-inference` | GPU Util | 75 | 10 | Triton Inference Server |
| `training` | GPU Util | 90 | 0 | Training jobs (no scale-to-zero) |
| `batch` | Memory % | 70 | 1 | Batch inference with aggressive scale-down |

## Quick Start

### Prerequisites

- Kubernetes cluster with NVIDIA GPU nodes
- [KEDA](https://keda.sh/docs/deploy/) installed
- NVIDIA drivers installed on GPU nodes

### Install

```bash
helm install keda-gpu-scaler deploy/helm/keda-gpu-scaler \
  --namespace keda \
  --set nodeSelector."nvidia\.com/gpu\.present"=true
```

### Create a ScaledObject

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: vllm-gpu-scaler
spec:
  scaleTargetRef:
    name: vllm-deployment
  minReplicaCount: 0
  maxReplicaCount: 8
  cooldownPeriod: 60
  triggers:
    - type: external
      metadata:
        scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
        profile: "vllm-inference"
```

### Custom Configuration

Override any profile default or use raw metrics:

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "gpu_utilization"
      targetValue: "85"
      activationThreshold: "10"
      gpuIndex: "0"           # specific GPU, or omit for all
      aggregation: "max"      # max, min, avg, sum across GPUs
```

## Configuration Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `profile` | Pre-built scaling profile name | (none) |
| `metricType` | GPU metric to scale on | `gpu_utilization` |
| `targetValue` | Target metric value | `80` |
| `targetGpuUtilization` | Shorthand for GPU util target | (none) |
| `targetMemoryUtilization` | Shorthand for memory util target | (none) |
| `activationThreshold` | Value below which scale-to-zero activates | `0` |
| `gpuIndex` | Specific GPU index to monitor | `-1` (all) |
| `aggregation` | How to aggregate across GPUs: `max`, `min`, `avg`, `sum` | `max` |
| `pollIntervalSeconds` | Metric polling interval | `10` |

## Building

```bash
# Build binary (requires CGO for NVML)
make build

# Build Docker image
make docker-build

# Run tests
make test

# Generate protobuf
make proto
```

## Why External Scaler (not a native KEDA scaler)?

1. **CGO requirement**: NVML's Go bindings (`go-nvml`) require CGO. KEDA builds with `CGO_ENABLED=0`.
2. **Node-level access**: The KEDA operator runs as a central pod. NVML requires local GPU device access, which only a DaemonSet on GPU nodes can provide.
3. **Independent release cycle**: Ship GPU scaling improvements without waiting for KEDA release cycles.

## Contributing

Contributions are welcome! Please open an issue or PR.

## License

Apache License 2.0. See [LICENSE](LICENSE).
