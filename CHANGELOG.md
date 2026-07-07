# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- **MIG (Multi-Instance GPU) per-instance metrics**. `CollectAll()` auto-detects MIG-enabled GPUs and enumerates each compute instance via NVML, returning one `Metrics` entry per instance instead of one per physical GPU. Shared chip-level metrics (temperature, power, PCIe, NVLink) are read from the parent GPU and copied into every instance. In HPC environments (SLURM, Flux), MIG UUIDs in `CUDA_VISIBLE_DEVICES` are detected and resolved individually via `CollectByUUID()`. New `Metrics` fields: `IsMIGInstance`, `ParentIndex`, `MigProfile`. New CSV columns: `is_mig_instance`, `parent_index`, `mig_profile`. Table output shows `gpu<N>/inst<M>` labels for MIG rows.
- **vLLM queue depth and KV cache usage scaling** (`pkg/vllm`). Reads pending request count directly from the vLLM engine's Prometheus `/metrics` endpoint instead of relying on GPU utilization as a proxy, for faster scaling reaction time ([#28](https://github.com/pmady/keda-gpu-scaler/issues/28)). New `vllm_queue_depth` (`vllm:num_requests_waiting`) and `vllm_kv_cache_usage` (`vllm:gpu_cache_usage_perc`) metric types, a new `vllmEndpoint` trigger parameter, and a `vllm-queue-depth` pre-built profile. vLLM clients are cached per endpoint on the scaler.

## [v0.5.0] - 2026-06-23

### Added

- **Cross-environment GPU metrics parity** (`--env` flag): Single binary and unified JSON schema across Kubernetes, SLURM, Flux, and standalone. The `pkg/env` package auto-detects the orchestrator (priority: SLURM → Flux → Kubernetes → standalone) and populates a common `environment` block in all output formats (JSON, CSV, table). Replaces the separate `--slurm` and `--flux` flags.
- Unified JSON output schema with top-level `environment` and `collected_at` fields for cross-environment GPU comparison.
- `pkg/env` package: `Detect()`, `Parse()`, `FromType()`, `Context` with `VisibleDevices()`, `Header()`, `Row()`.
- Kubernetes environment detection via `KUBERNETES_SERVICE_HOST`; pod/node/namespace metadata via Downward API env vars.
- Runtime environment metadata logged at startup (orchestrator, node, pod, namespace).
- `docs/cross-env-comparison.md`: Guide for comparing GPU performance across on-prem and cloud.
- CI: arm64 builds, release checksums, semver tag guard rail.

### Changed

- Bumped `github.com/NVIDIA/go-nvml` to 0.13.2-0.

### Contributors

- [@venkata22a](https://github.com/venkata22a) - Cross-environment metrics, Flux integration, CI/CD improvements

[v0.5.0]: https://github.com/pmady/keda-gpu-scaler/compare/v0.4.0...v0.5.0

## [v0.4.0] - 2026-06-08

### Added

- HTTP health probes (`--probe-port=8081`) with `/healthz` and `/readyz` endpoints
- PCIe bandwidth metrics: `pcie_tx_kbps`, `pcie_rx_kbps` for CPU↔GPU throughput monitoring
- NVLink bandwidth metrics: `nvlink_tx_mbps`, `nvlink_rx_mbps` for GPU↔GPU communication monitoring
- `distributed-training` scaling profile optimized for NVLink systems
- Standalone `gpu-metrics` CLI for HPC environments (SLURM, Flux, bare metal)
- SLURM workload manager integration for gpu-metrics CLI
- Updated Prometheus metrics with PCIe/NVLink throughput gauges and device count
- Support for 10 total metric types including temperature, power draw, and memory metrics
- Graceful handling of non-NVLink hardware (metrics return 0 with debug logging)

### Changed

- Updated metricType parameter documentation with complete supported values table
- Enhanced scaling profiles table from 4 to 5 profiles
- Improved test coverage with dedicated PCIe/NVLink test functions

### Contributors

- [@ibobgunardi](https://github.com/ibobgunardi) - HTTP health probes implementation
- [@venkata22a](https://github.com/venkata22a) - PCIe/NVLink bandwidth metrics, CI/CD, and comprehensive documentation

[v0.4.0]: https://github.com/pmady/keda-gpu-scaler/compare/v0.3.0...v0.4.0

## [v0.3.0] - 2026-05-29

### Added

- Optional Prometheus metrics endpoint (`--metrics-port=9090`, set to 0 to disable)
- Per-GPU Prometheus gauges: utilization, memory, temperature, power draw
- Scaler operational metrics: collection counters, duration histogram, gRPC request counters
- `InstrumentedCollector` wrapper for transparent metrics collection
- `/healthz` HTTP health check endpoint (when metrics enabled)
- Helm values: `metrics.enabled` and `metrics.port`
- Unit tests for `pkg/metrics` package

## [v0.2.0] - 2026-05-25

### Added

- GPU collector package tests (`pkg/gpu/collector_test.go`) — MockCollector interface compliance, boundary conditions, empty device handling

### Changed

- Dependabot updates: grpc 1.81.1, zap 1.28.0, golangci-lint-action v9, actions/checkout v6, actions/setup-go v6, docker/login-action v4, docker/build-push-action v7

[v0.2.0]: https://github.com/pmady/keda-gpu-scaler/compare/v0.1.0...v0.2.0

## [v0.1.0] - 2026-05-19

### Added

- KEDA External Scaler gRPC server implementing `externalscaler.ExternalScalerServer`
- Direct NVML GPU metrics collection via `go-nvml` C-bindings
- 6 GPU metrics: utilization, memory utilization, memory used (MiB and %), temperature, power draw
- Pre-built scaling profiles: `vllm-inference`, `triton-inference`, `training`, `batch`
- Multi-GPU aggregation: `max`, `min`, `avg`, `sum`
- Scale-to-zero support via KEDA activation thresholds
- Per-GPU index targeting (`gpuIndex` parameter)
- Mock GPU collector for development and testing without hardware
- DaemonSet deployment manifests and Helm chart
- Unit tests for profiles, metric aggregation, and gRPC server
- E2E tests for full gRPC scaling path (no GPU required)
- CI pipeline: build, unit tests, e2e tests, lint, Helm lint, Docker build + push
- OpenSSF Best Practices badge

[v0.1.0]: https://github.com/pmady/keda-gpu-scaler/releases/tag/v0.1.0
