# Architecture

How keda-gpu-scaler works under the hood.

![Architecture](docs/images/architecture.png)

## Components

### DaemonSet (keda-gpu-scaler)

Runs on every node with `nvidia.com/gpu.present: "true"` label. Each pod:

- Loads `libnvidia-ml.so` via cgo
- Polls NVML every 2 seconds for GPU metrics
- Caches metrics in memory
- Serves gRPC on port 6000
- Optionally exposes Prometheus metrics on port 9090

### KEDA Operator

Standard KEDA deployment. Connects to the DaemonSet's gRPC endpoint via the `external` trigger type in ScaledObject.

### HPA

KEDA creates and manages an HPA resource. The HPA scales the target deployment based on the GPU metric value.

## Data Flow

```
GPU Hardware
    ↓
libnvidia-ml.so (NVML)
    ↓
DaemonSet (NVML poller, 2s loop)
    ↓
gRPC server (:6000)
    ↓
KEDA operator (GetMetrics call)
    ↓
HPA (scale decision)
    ↓
Target Deployment (replica count change)
```

## Why This Design

### DaemonSet (not Deployment)

NVML requires access to `/dev/nvidia*` device files. These only exist on the physical GPU node. A centralized Deployment can't read hardware state from remote nodes.

### gRPC (not HTTP/Prometheus)

KEDA's external scaler protocol is gRPC. Type-safe via protobuf, supports streaming for push-based activation, lower latency than HTTP scrape-and-parse.

### CGO (not pure Go)

NVIDIA's go-nvml library wraps the C NVML library. There's no pure-Go alternative that provides the same metrics. This is why GPU support can't be added to KEDA core (which builds with `CGO_ENABLED=0`).

## Ports

| Port | Protocol | Purpose |
|------|----------|---------|
| 6000 | gRPC | KEDA external scaler interface; also serves the gRPC Health Checking Protocol (`grpc.health.v1.Health`), reflecting NVML availability |
| 9090 | HTTP | Prometheus metrics (optional) |
| 8081 | HTTP | Legacy health probes (`/healthz`, `/readyz`), used when `probes.type: http` |

## Resource Requirements

Minimal footprint per node:

- CPU: ~10m idle, ~50m during metric collection
- Memory: ~20Mi
- No persistent storage

## Security

- Read-only NVML calls (no device configuration)
- ClusterIP service (not exposed outside cluster)
- No secrets or credentials required
- Minimal RBAC (just needs to read its own ConfigMap)

## More Details

See [docs/DESIGN.md](docs/DESIGN.md) for the full design document including scaling profiles, aggregation strategies, and testing approach.
