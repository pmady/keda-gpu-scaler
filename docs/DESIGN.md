# Design Document: keda-gpu-scaler

## Problem Statement

GPU inference workloads on Kubernetes cannot be autoscaled using standard HPA. CPU and memory metrics are irrelevant — a vLLM pod serving 200 concurrent requests shows 8% CPU while the GPU is 100% saturated. The existing approach chains dcgm-exporter → Prometheus → PromQL → KEDA, adding 5 components and 15-30 seconds of metric latency.

The goal: scale GPU workloads from hardware metrics with sub-second latency, no metrics pipeline, and no PromQL.

## Why an External Scaler (Not a Native KEDA Scaler)

Three hard constraints make embedding GPU support inside KEDA core impossible:

### 1. CGO Constraint

NVIDIA's Go bindings ([go-nvml](https://github.com/NVIDIA/go-nvml)) call into `libnvidia-ml.so` via cgo. KEDA builds its operator with `CGO_ENABLED=0` for portability — every binary is a static Linux ELF. Adding a cgo dependency would break KEDA's entire build and release pipeline.

This isn't a temporary limitation. It's a fundamental incompatibility between how KEDA ships binaries and how NVIDIA's library works.

### 2. Node-Level Hardware Access

NVML reads GPU state through `/dev/nvidiactl` and `/dev/nvidia0..N`. These device files are only available on the physical GPU node. The KEDA operator runs as a single centralized Deployment — it has no access to GPU devices on worker nodes.

The only correct Kubernetes pattern for node-level hardware polling is a **DaemonSet**. Each instance runs on a GPU node, mounts the NVIDIA device files, and serves metrics locally.

### 3. Independent Release Cycle

GPU infrastructure moves fast. Tying GPU scaling features to KEDA's release cadence (which needs to coordinate across 50+ scalers) would slow iteration. As a standalone component, we can ship fixes and new GPU metrics in hours, not months.

This design was discussed and documented in [KEDA issue #7538](https://github.com/kedacore/keda/issues/7538).

## Architecture

```
GPU Node                                    KEDA Operator
┌──────────────────────────────┐           ┌──────────────────┐
│  DaemonSet: keda-gpu-scaler  │           │                  │
│                              │           │  ExternalScaler  │
│  ┌────────────┐              │  gRPC     │  trigger config  │
│  │ NVML poller│──metrics──►  │──:6000──► │                  │
│  │ (2s loop)  │              │           │  → HPA decision  │
│  └────────────┘              │           │  → scale up/down │
│       ↕                      │           └──────────────────┘
│  libnvidia-ml.so             │
│  /dev/nvidia0..N             │
└──────────────────────────────┘
```

### Data Flow

1. The DaemonSet starts an NVML polling loop (default 2 seconds)
2. Each cycle reads: SM utilization, memory controller utilization, VRAM used/total, temperature, power draw
3. Metrics are cached in memory (no disk, no external store)
4. KEDA calls `GetMetrics()` over gRPC on the `externalscaler.ExternalScalerServer` interface
5. The scaler returns the requested metric with the aggregation method specified in the ScaledObject
6. KEDA feeds the metric value into HPA for a scale up/down/to-zero decision
7. (Optional) An HTTP `/metrics` endpoint on port 9090 exposes Prometheus gauges for GPU fleet monitoring — independent of the KEDA scaling path

### gRPC Interface

The scaler implements four methods from KEDA's ExternalScaler protobuf contract:

| Method | Purpose |
|--------|---------|
| `IsActive` | Returns true if any GPU metric exceeds the activation threshold (enables scale-from-zero) |
| `StreamIsActive` | Streaming version of IsActive for push-based activation |
| `GetMetricSpec` | Returns the metric name and target value for HPA |
| `GetMetrics` | Returns the current GPU metric value |

### Why gRPC (Not HTTP Metrics)

KEDA's external scaler protocol is gRPC by design — type-safe via protobuf (no PromQL string parsing), supports streaming for push-based activation, and lower latency than HTTP scrape-and-parse.

## Scaling Profiles

Raw metric thresholds are error-prone if you don't know what "80% GPU utilization" means for your workload. Profiles encode reasonable defaults:

| Profile | What it optimizes for |
|---------|----------------------|
| `vllm-inference` | LLM serving. Scales on VRAM pressure (80%) because vLLM pre-allocates KV cache. Activation threshold at 5% for scale-to-zero. |
| `triton-inference` | Multi-model serving. Scales on SM utilization (75%) because Triton shares GPU across models. Higher activation (10%) to avoid flapping. |
| `training` | Batch training. Scales on SM utilization (90%) with no scale-to-zero (activation 0) to avoid killing checkpoints. |
| `batch` | Offline batch inference. Aggressive scale-down with 70% memory threshold and low activation (1%). |

Users can override any profile parameter in the ScaledObject metadata.

## Multi-GPU Aggregation

Nodes with 4-8 GPUs need an aggregation strategy. The `aggregation` parameter controls how per-GPU metrics are combined into a single scalar for KEDA:

- **max** (default): Scale when any GPU hits the threshold. Best for inference where hot GPUs indicate overload.
- **avg**: Scale on average utilization. Best for training where GPUs should be evenly loaded.
- **min**: Scale when the least-loaded GPU hits the threshold. Conservative.
- **sum**: Total utilization across all GPUs. Useful for capacity-based scaling.

## Testing Strategy

### Unit Tests (no GPU required)

All metric parsing, aggregation, and profile resolution logic is unit-tested with a mock NVML implementation (`pkg/gpu/mock.go`). The mock returns configurable metric values for any number of simulated GPUs.

### E2E Tests (no GPU required)

The gRPC server is tested end-to-end using the mock collector. Tests verify the full path: ScaledObject metadata → metric extraction → gRPC response → activation check.

### Manual GPU Testing

For real hardware validation, deploy to a GPU cluster and verify:
```bash
# Check scaler logs
kubectl logs -n keda -l app=keda-gpu-scaler

# Verify KEDA sees the external scaler
kubectl get scaledobject -A -o yaml | grep -A5 external
```

## Cross-Environment GPU Metrics (`pkg/env`)

The `gpu-metrics` CLI targets HPC and cloud environments equally. The `pkg/env` package centralises all orchestrator detection and metadata so the rest of the codebase never branches on scheduler type.

### Environment detection

`env.Detect()` inspects process environment variables in priority order:

1. `SLURM_JOB_ID` → SLURM
2. `FLUX_JOB_ID` → Flux
3. `KUBERNETES_SERVICE_HOST` → Kubernetes (injected by kubelet into every pod)
4. Otherwise → Standalone

`env.Parse(flagValue)` converts the `--env` flag string to an `env.Type`. The value `"auto"` triggers `Detect()`.

### Unified Context

`env.FromType(t)` returns an `env.Context` struct with orchestrator-agnostic fields (`Orchestrator`, `NodeName`, `JobID`, `TaskRank`) plus environment-specific extras (`Partition` for SLURM, `FluxURI` for Flux, `PodName`/`Namespace` for Kubernetes). All fields are JSON-serialised into the top-level `environment` block of every output document.

The unexported `visibleDevices []int` field carries the scheduler-assigned GPU indices so `gpu-metrics` can restrict NVML collection to the right devices without any scheduler-specific code in the CLI itself.

### Unified JSON schema

Before this package, the JSON output had a `slurm` or `flux` top-level key that changed depending on the runtime. Any downstream parser had to branch on which key was present. The new schema is always:

```
{ "environment": { ... }, "collected_at": "...", "devices": [...] }
```

This makes cross-environment comparison trivially composable with `jq`, pandas, or any streaming JSON processor.

### Kubernetes Downward API

For Kubernetes, `NODE_NAME`, `POD_NAME`, and `POD_NAMESPACE` must be exposed via the Downward API (not auto-set by the runtime). The deployment manifests and Helm chart include these mappings so the metadata is populated out of the box.

## Security Considerations

- The DaemonSet needs read-only access to NVIDIA device files — no cluster-wide RBAC
- The gRPC port (6000) is exposed only as a ClusterIP Service — not reachable outside the cluster
- The metrics port (9090) is optional and can be disabled entirely with `--metrics-port=0`
- No secrets or credentials are required
- NVML calls are read-only (metrics collection, no device configuration)

## MIG (Multi-Instance GPU) Support (`pkg/gpu`)

NVIDIA A100 and H100 GPUs can be partitioned into 2–7 independent MIG instances, each with its own NVML handle, dedicated SM engines, a fixed framebuffer slice, and independent utilization / memory counters. The scaler supports MIG via two collection strategies.

### Kubernetes / Standalone — automatic enumeration

`CollectAll()` checks each physical GPU for MIG mode via `nvmlDeviceGetMigMode`. When MIG is active, instead of reading the physical GPU once, it calls `nvmlDeviceGetMigDeviceHandleByIndex` in a loop until the driver returns an error, collecting one `Metrics` per compute instance. The loop replaces the physical-GPU entry entirely; the physical GPU does not appear as a separate row.

Certain metrics (temperature, power draw/limit, PCIe throughput, NVLink bandwidth) are chip-level resources shared across all slices. They are read once from the physical handle via `collectPhysicalForMIG()` and copied verbatim into every instance entry.

### HPC (SLURM / Flux) — UUID-based collection

When a job is allocated MIG instances, the driver writes MIG UUIDs to `CUDA_VISIBLE_DEVICES` (e.g. `MIG-GPU-aaaa1111.../3/0`). The `pkg/slurm` and `pkg/flux` `MIGUUIDs()` methods detect these entries (they start with `"MIG-"`) and return them separately from integer device indices. `pkg/env.FromType()` propagates them into `Context.migUUIDs`, and `cmd/gpu-metrics` calls `CollectByUUID()` for each. `nvmlDeviceGetHandleByUUID` resolves MIG UUIDs correctly — the same path used for standard GPU UUIDs.

### MIG Metrics struct fields

Three fields are added to `gpu.Metrics`; they are zero-valued for standard (non-MIG) entries:

| Field | Type | Meaning |
|-------|------|---------|
| `IsMIGInstance` | `bool` | `true` when this entry is a MIG compute instance |
| `ParentIndex` | `int` | Physical GPU index (`-1` when resolved by UUID) |
| `MigProfile` | `string` | Profile slice, e.g. `"3g.40gb"` |

### Degraded states

When MIG mode is enabled but the GPU has not yet been partitioned (no compute instances exist), `collectMIGInstances` logs a warning and returns no entries for that GPU — rather than fabricating physical-level metrics that would misrepresent the partitioned state.

`CollectDevice(index)` still works on MIG-enabled GPUs but logs a warning and returns physical-level metrics only. Use `CollectAll` or `CollectByUUID` for per-instance MIG data.

## Future Work

- **AMD ROCm support**: Same DaemonSet pattern, different hardware library (`rocm-smi`)
- **NVLink topology**: Prefer scaling on nodes with direct GPU-to-GPU interconnect
- **vLLM queue depth**: Read pending request count directly from vLLM's engine API for more precise scaling
