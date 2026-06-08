# Configuration

Everything goes in the ScaledObject trigger `metadata`. No config files or extra CRDs needed.

## Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `profile` | Pre-built scaling profile name | (none) |
| `metricType` | GPU metric to scale on: `gpu_utilization`, `memory_utilization`, `memory_used_mib`, `memory_used_percent`, `temperature`, `power_draw`, `pcie_tx_kbps`, `pcie_rx_kbps`, `nvlink_tx_mbps`, `nvlink_rx_mbps` | `gpu_utilization` |
| `targetValue` | Target metric value for scaling | `80` |
| `targetGpuUtilization` | Shorthand for GPU utilization target | (none) |
| `targetMemoryUtilization` | Shorthand for VRAM utilization target | (none) |
| `activationThreshold` | Value below which scale-to-zero activates | `0` |
| `gpuIndex` | Specific GPU index to monitor | `-1` (all GPUs) |
| `aggregation` | Multi-GPU aggregation: `max`, `min`, `avg`, `sum` | `max` |
| `pollIntervalSeconds` | Metric polling interval | `10` |

## Scaling Profiles

Profiles bundle defaults for common workloads. Override any parameter in the trigger metadata.

| Profile | Primary Metric | Target | Activation | Use Case |
|---------|---------------|--------|------------|----------|
| `distributed-training` | NVLink TX | 800 | 100 | Data-parallel training on NVLink systems |
| `vllm-inference` | Memory % | 80 | 5 | vLLM / LLM serving with scale-to-zero |
| `triton-inference` | GPU Util | 75 | 10 | NVIDIA Triton Inference Server |
| `training` | GPU Util | 90 | 0 | Training jobs (no scale-to-zero) |
| `batch` | Memory % | 70 | 1 | Batch inference with aggressive scale-down |

### Using a profile

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "vllm-inference"
```

### Overriding a profile parameter

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "vllm-inference"
      targetValue: "90"          # override the default 80
```

### Using raw metrics (no profile)

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "gpu_utilization"
      targetValue: "85"
      activationThreshold: "10"
      gpuIndex: "0"
      aggregation: "max"
```

## Multi-GPU Aggregation

On multi-GPU nodes, `aggregation` controls how per-GPU values are reduced to one number:

- **max** (default) — scale when any GPU hits the threshold. Good for inference where one hot GPU means overload.
- **avg** — scale on average utilization. Good for training where GPUs should be evenly loaded.
- **min** — scale when the least-loaded GPU hits the threshold. Conservative.
- **sum** — total utilization. Useful for capacity-based decisions.

## Advanced GPU Metrics

### PCIe Bandwidth Metrics
- `pcie_tx_kbps` - PCIe transmit throughput (KB/s) - useful for CPU↔GPU data transfer bottlenecks
- `pcie_rx_kbps` - PCIe receive throughput (KB/s) - useful for GPU↔CPU data transfer bottlenecks

### NVLink Bandwidth Metrics  
- `nvlink_tx_mbps` - NVLink transmit throughput (MB/s) - useful for GPU↔GPU communication in distributed training
- `nvlink_rx_mbps` - NVLink receive throughput (MB/s) - useful for GPU↔GPU communication in distributed training

**Note:** NVLink metrics are 0 on hardware without NVLink (e.g., T4, A10 GPUs). This is normal behavior.

## Scale-to-Zero

Set `activationThreshold` to enable scale-to-zero. When all GPU metrics drop below this value, KEDA reports the scaler as inactive and scales the deployment to zero replicas.

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "gpu_utilization"
      targetValue: "80"
      activationThreshold: "5"    # scale to zero when GPU util < 5%
```

## Server Flags

These flags configure the scaler binary itself (passed via `args` in the DaemonSet or Helm values):

| Flag | Description | Default |
|------|-------------|---------|
| `--port` | gRPC server port | `6000` |
| `--metrics-port` | Prometheus HTTP metrics port (0 to disable) | `9090` |
| `--probe-port` | Liveness/readiness HTTP probe port (0 to disable) | `8081` |
| `--log-level` | Log level: `debug`, `info`, `warn`, `error` | `info` |

### Helm Values

```yaml
grpc:
  port: 6000

metrics:
  enabled: true    # set to false to disable Prometheus endpoint
  port: 9090

probes:
  enabled: true
  port: 8081

logLevel: info
```

## Prometheus Metrics

When `--metrics-port` is non-zero, an HTTP server exposes `/metrics` in Prometheus format. This is optional and does not affect the KEDA scaling path.

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `keda_gpu_scaler_gpu_utilization_percent` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | GPU compute utilization |
| `keda_gpu_scaler_gpu_memory_used_bytes` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | GPU memory in use |
| `keda_gpu_scaler_gpu_memory_total_bytes` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | Total GPU memory |
| `keda_gpu_scaler_gpu_temperature_celsius` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | GPU temperature |
| `keda_gpu_scaler_gpu_power_draw_watts` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | GPU power draw |
| `keda_gpu_scaler_gpu_pcie_tx_throughput_kbps` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | PCIe transmit throughput |
| `keda_gpu_scaler_gpu_pcie_rx_throughput_kbps` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | PCIe receive throughput |
| `keda_gpu_scaler_gpu_nvlink_tx_throughput_mbps` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | NVLink transmit throughput |
| `keda_gpu_scaler_gpu_nvlink_rx_throughput_mbps` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name` | NVLink receive throughput |
| `keda_gpu_scaler_collections_total` | Counter | — | Total NVML collection calls |
| `keda_gpu_scaler_collection_errors_total` | Counter | — | Failed NVML collections |
| `keda_gpu_scaler_collection_duration_seconds` | Histogram | — | NVML collection latency |
| `keda_gpu_scaler_scaler_requests_total` | Counter | `method` | gRPC requests by method |
| `keda_gpu_scaler_scaler_request_errors_total` | Counter | `method` | gRPC errors by method |

## Kubernetes Probes

When `--probe-port` is non-zero, an HTTP server exposes:

- `/healthz` — returns 200 while the scaler process is alive.
- `/readyz` — returns 200 after NVML initializes and the first metrics collection succeeds.

## Examples

### Distributed Training on NVLink

```yaml
# Example: Distributed training on NVLink systems
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "distributed-training"
      # Override if needed:
      # targetValue: "1000"  # Higher target for faster hardware
      # activationThreshold: "50"
```

Check `deploy/examples/` for ScaledObject manifests:

- `vllm-scaledobject.yaml` — vLLM inference with scale-to-zero
- `custom-gpu-utilization.yaml` — raw GPU utilization scaling
