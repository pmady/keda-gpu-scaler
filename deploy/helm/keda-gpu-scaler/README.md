# keda-gpu-scaler

A [KEDA](https://keda.sh) external scaler that reads NVIDIA GPU metrics via NVML
and exposes them to KEDA, so you can autoscale GPU workloads (vLLM, Triton,
training, batch) on GPU utilization, memory, temperature, power draw, or
PCIe/NVLink throughput.

The chart deploys the scaler as a **DaemonSet** on your GPU nodes plus a Service
that KEDA `ScaledObject`s point at through an `external` trigger.

## Prerequisites

- Kubernetes 1.21+
- [KEDA](https://keda.sh/docs/latest/deploy/) installed in the cluster
- NVIDIA GPU nodes with the driver (`libnvidia-ml.so`) available — either via the
  NVIDIA container runtime (the default, `runtimeClassName: nvidia`) or via
  host-path mounts (`nvmlHostMounts.enabled: true`)

## Installing the Chart

```bash
helm install keda-gpu-scaler ./deploy/helm/keda-gpu-scaler \
  --namespace keda --create-namespace
```

Override values inline or with a values file:

```bash
helm install keda-gpu-scaler ./deploy/helm/keda-gpu-scaler \
  --set logLevel=debug --set metrics.port=9100
```

## Uninstalling the Chart

```bash
helm uninstall keda-gpu-scaler --namespace keda
```

## Usage

Once installed, point a KEDA `ScaledObject` at the scaler Service. See
`examples/scaledobject.yaml` for a complete vLLM example:

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "vllm-inference"
```

Available profiles: `vllm-inference`, `vllm-queue-depth`, `triton-inference`,
`training`, `batch`, `distributed-training`.

`vllm-queue-depth` scales on pending requests read from the vLLM engine's own
metrics endpoint rather than GPU utilization/memory, and requires a
`vllmEndpoint` trigger parameter pointing at it (e.g.
`http://vllm-deployment:8000/metrics`) — see the repo's
`docs/configuration.md#vllm-engine-metrics`.

## Parameters

### Image

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `image.repository` | string | `ghcr.io/pmady/keda-gpu-scaler` | Image repository. |
| `image.pullPolicy` | string | `IfNotPresent` | Image pull policy (`Always`, `IfNotPresent`, or `Never`). |
| `image.tag` | string | `""` | Image tag. Defaults to the chart `appVersion` when left empty. |
| `imagePullSecrets` | list | `[]` | Names of existing image pull secrets for pulling from a private registry. |

### Naming & ServiceAccount

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `nameOverride` | string | `""` | Override the chart name used when generating resource names. |
| `fullnameOverride` | string | `""` | Override the fully qualified app name used for all resources. |
| `serviceAccount.create` | bool | `true` | Create a dedicated ServiceAccount for the scaler. |
| `serviceAccount.annotations` | object | `{}` | Annotations to add to the ServiceAccount (e.g. IRSA / Workload Identity). |
| `serviceAccount.name` | string | `""` | Name of the ServiceAccount to use. Generated from the fullname when empty. |

### Networking

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `service.type` | string | `ClusterIP` | Service type for the gRPC (and metrics) endpoint. |
| `service.port` | int | `6000` | Service port, forwarded to the container's gRPC port. |
| `grpc.port` | int | `6000` | Container port the gRPC external-scaler server listens on (passed as `--port`). |

### Observability

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `logLevel` | string | `info` | Log verbosity: `debug`, `info`, `warn`, or `error` (passed as `--log-level`). |
| `metrics.enabled` | bool | `true` | Enable the Prometheus metrics endpoint. When false, the scaler runs with `--metrics-port=0` and no metrics container port or Service entry is created. |
| `metrics.port` | int | `9090` | Port for the Prometheus metrics HTTP server (passed as `--metrics-port`). |
| `probes.enabled` | bool | `true` | Enable the health/readiness HTTP server and the pod liveness/readiness probes. When false, the scaler runs with `--probe-port=0` and no probes are set. |
| `probes.port` | int | `8081` | Port for the health/readiness HTTP server (passed as `--probe-port`). |

### GPU access

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `securityContext` | object | `{privileged: true, runAsUser: 0}` | Container security context. NVML needs privileged access to the GPU device files. |
| `runtimeClassName` | string | `nvidia` | RuntimeClass for the pods. The NVIDIA container runtime mounts the GPU devices automatically. Set to `""` if your cluster has no `nvidia` runtime class (and enable `nvmlHostMounts` instead). |
| `nvmlHostMounts.enabled` | bool | `false` | Enable hostPath mounts of the NVML device files and shared library, for clusters without the nvidia runtime class. |
| `nvmlHostMounts.nvidiactl` | string | `/dev/nvidiactl` | Host path to the NVIDIA control device. |
| `nvmlHostMounts.nvidiaUvm` | string | `/dev/nvidia-uvm` | Host path to the NVIDIA Unified Memory device. |
| `nvmlHostMounts.nvmlLib` | string | `/usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1` | Host path to the NVML shared library. |

### Scheduling & workload

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `nodeSelector` | object | `{nvidia.com/gpu.present: "true"}` | Node selector for scheduling scaler pods. Defaults to NVIDIA GPU nodes. |
| `tolerations` | list | tolerate `nvidia.com/gpu:NoSchedule` | Tolerations for scheduling scaler pods. Defaults to tolerating the standard NVIDIA GPU taint. |
| `affinity` | object | `{}` | Affinity rules for scheduling scaler pods. |
| `resources` | object | `{limits: {cpu: 200m, memory: 128Mi}, requests: {cpu: 100m, memory: 64Mi}}` | CPU/memory resource requests and limits for the scaler container. |
| `podAnnotations` | object | `{}` | Annotations to add to the scaler pods. |
| `updateStrategy` | object | `{type: RollingUpdate, rollingUpdate: {maxUnavailable: 1}}` | DaemonSet update strategy. |
| `terminationGracePeriodSeconds` | int | `30` | Grace period (in seconds) for a pod to shut down before it is force-killed. |
