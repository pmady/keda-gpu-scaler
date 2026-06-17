# Roadmap

Technical direction for keda-gpu-scaler. Updated as priorities shift.

## Current (v0.4.x)

- NVIDIA GPU support via NVML
- Pre-built scaling profiles (vLLM, Triton, training, batch)
- Helm chart deployment
- Prometheus metrics endpoint
- SLURM and Flux workload manager integration
- PCIe and NVLink throughput metrics

## Next (v0.5.x)

- ✅ **Cross-environment GPU metrics parity** — unified `--env` flag and JSON schema across Kubernetes, SLURM, Flux, and standalone. Compare GPU performance across on-prem and cloud with the same binary and tooling. ([#54](https://github.com/pmady/keda-gpu-scaler/issues/54))
- ✅ **MIG support** — Per-instance metrics for Multi-Instance GPU partitions. `CollectAll` enumerates MIG compute instances automatically; HPC environments resolve MIG UUIDs via `CollectByUUID`. ([#26](https://github.com/pmady/keda-gpu-scaler/issues/26))
- **vLLM queue depth** — Scale on pending requests via vLLM engine API
- **Improved aggregation** — Weighted averages, percentile-based thresholds

## Future

- **AMD ROCm** — Same DaemonSet pattern with rocm-smi bindings
- **Intel Gaudi** — Habana Management Library integration
- **Multi-cluster** — Federated scaling decisions across GPU clusters
- **Cost-aware scaling** — Factor in spot/preemptible pricing

## Non-Goals

- Replacing DCGM exporter for observability (use both — this project is for scaling, not dashboards)
- GPU sharing/virtualization (use HAMi, MIG, or time-slicing instead)
- Node-level autoscaling (use Karpenter or Cluster Autoscaler)

## How to Influence

Open an issue or discussion. If you're running into a real problem, that moves things up the list.
