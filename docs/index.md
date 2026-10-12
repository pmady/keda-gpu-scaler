# keda-gpu-scaler

**Scale Kubernetes GPU workloads from real hardware metrics. No Prometheus. No DCGM. No PromQL.**

A [KEDA External Scaler](https://keda.sh/docs/latest/concepts/external-scalers/) that reads NVIDIA GPU metrics directly from NVML C-bindings and autoscales your vLLM, Triton, and custom inference deployments — including scale-to-zero.

```
GPU Node                          KEDA Operator
┌─────────────────────┐           ┌──────────────────┐
│ keda-gpu-scaler     │──gRPC───> │ External Scaler  │
│ (DaemonSet)         │           │ trigger          │
│                     │           └────────┬─────────┘
│ NVML: 92% GPU util  │                    │
│ NVML: 14.2GB VRAM   │           Scale vllm-deployment
└─────────────────────┘           from 3 → 8 replicas
```

## Why This Exists

Scaling AI inference on Kubernetes using CPU/Memory HPA is broken. Your GPU nodes sit at 10% CPU while the GPUs are 100% saturated with 200+ pending requests in the vLLM queue.

```
BEFORE: GPU Pod → dcgm-exporter → Prometheus → PromQL → KEDA → HPA
        (5 components, 15-30s scrape delay, PromQL queries break on upgrades)

AFTER:  GPU Pod → keda-gpu-scaler (NVML) → KEDA → HPA
        (2 components, sub-second metrics, zero configuration)
```

## Docs

- [Getting Started](getting-started.md)
- [Architecture & Design](DESIGN.md)
- [Migrating from dcgm-exporter](MIGRATION.md)
- [Configuration Reference](configuration.md)
- [HPC & Cross-Environment Metrics](hpc.md)
- [Cross-Environment Comparison Guide](cross-env-comparison.md)
- [FAQ](FAQ.md)
- [Contributing](contributing.md)

## GPU Metrics

| Metric | Description | Unit |
|--------|-------------|------|
| `gpu_utilization` | GPU compute (SM) utilization | % (0-100) |
| `memory_utilization` | GPU memory controller utilization | % (0-100) |
| `memory_used_mib` | GPU VRAM used | MiB |
| `memory_used_percent` | GPU VRAM used as percentage of total | % (0-100) |
| `temperature` | GPU die temperature | Celsius |
| `power_draw` | GPU power consumption | Watts |

## Featured In

- [GPU Autoscaling on Kubernetes with KEDA — Building an External Scaler](https://www.cncf.io/blog/2026/05/27/gpu-autoscaling-on-kubernetes-with-keda-building-an-external-scaler/) — CNCF Blog
- [Abstracting AI Infrastructure: Native GPU Scaling for Internal Developer Platforms](https://platformengineering.com/contributed-content/abstracting-ai-infrastructure-native-gpu-scaling-for-internal-developer-platforms/) — Platform Engineering
- [The Financial Trap of Autonomous Networks: Scaling Agentic AI in the Telecom Core](https://techblog.comsoc.org/2026/03/30/the-financial-trap-of-autonomous-networks-scaling-agentic-ai-in-the-telecom-core/) — IEEE ComSoc Technology Blog
