# Roadmap

Technical direction for keda-gpu-scaler. Updated as priorities shift.

## v0.5.0 (Released June 2026)

- ✅ Cross-environment GPU metrics parity unified `--env` flag and JSON schema across Kubernetes, SLURM, Flux, and standalone ([#54](https://github.com/pmady/keda-gpu-scaler/issues/54))
- ✅ Flux workload manager integration ([#53](https://github.com/pmady/keda-gpu-scaler/issues/53))
- ✅ SLURM workload manager integration ([#52](https://github.com/pmady/keda-gpu-scaler/issues/52))
- ✅ CI hardening: arm64 builds, release checksums, semver tag guard

## v0.4.0 (Released May 2026)

- NVIDIA GPU support via NVML
- Pre-built scaling profiles (vLLM, Triton, training, batch, distributed-training)
- Helm chart deployment
- Prometheus metrics endpoint
- PCIe and NVLink throughput metrics
- HTTP health probes

## Next (v0.6.0 — August 2026)

- ✅ **MIG support** — Per-instance metrics for Multi-Instance GPU partitions. `CollectAll` enumerates MIG compute instances automatically; HPC environments resolve MIG UUIDs via `CollectByUUID`. ([#26](https://github.com/pmady/keda-gpu-scaler/issues/26))
- **New scaling profiles** — TGI, Ollama ([#64](https://github.com/pmady/keda-gpu-scaler/issues/64), [#65](https://github.com/pmady/keda-gpu-scaler/issues/65))
- **vLLM queue depth** — Scale on pending requests via vLLM engine API ([#28](https://github.com/pmady/keda-gpu-scaler/issues/28))
- **Improved aggregation** — p95, p99 percentile methods ([#69](https://github.com/pmady/keda-gpu-scaler/issues/69))
- **CI/CD hardening** — golangci-lint config, go vet, test coverage, pre-commit hooks ([#72](https://github.com/pmady/keda-gpu-scaler/issues/72), [#73](https://github.com/pmady/keda-gpu-scaler/issues/73), [#74](https://github.com/pmady/keda-gpu-scaler/issues/74), [#76](https://github.com/pmady/keda-gpu-scaler/issues/76))
- **Grafana dashboard** for GPU fleet visibility ([#29](https://github.com/pmady/keda-gpu-scaler/issues/29))
- **Contributor experience** — CONTRIBUTORS.md, --version flag, --dry-run flag ([#70](https://github.com/pmady/keda-gpu-scaler/issues/70), [#62](https://github.com/pmady/keda-gpu-scaler/issues/62), [#71](https://github.com/pmady/keda-gpu-scaler/issues/71))

## Future

- **AMD ROCm**: Same DaemonSet pattern with rocm-smi bindings
- **Intel Gaudi**: Habana Management Library integration
- **Multi-cluster**: Federated scaling decisions across GPU clusters
- **Cost-aware scaling**: Factor in spot/preemptible pricing

## Non-Goals

- Replacing DCGM exporter for observability (use both this project is for scaling, not dashboards)
- GPU sharing/virtualization (use HAMi, MIG, or time-slicing instead)
- Node-level autoscaling (use Karpenter or Cluster Autoscaler)

## How to Influence

Open an issue or discussion. If you're running into a real problem, that moves things up the list.
