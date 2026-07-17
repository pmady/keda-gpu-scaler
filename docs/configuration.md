# Configuration

Everything goes in the ScaledObject trigger `metadata`. No config files or extra CRDs needed.

## Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `profile` | Pre-built scaling profile name | (none) |
| `metricType` | GPU metric to scale on (see table below) | `gpu_utilization` |
| `targetValue` | Target metric value for scaling | `80` |
| `targetGpuUtilization` | Shorthand for GPU utilization target | (none) |
| `targetMemoryUtilization` | Shorthand for VRAM utilization target | (none) |
| `activationThreshold` | Value below which scale-to-zero activates | `0` |
| `gpuIndex` | Specific GPU index to monitor. Must be `-1` (all GPUs) or `>= 0`; other negative values are rejected | `-1` (all GPUs) |
| `aggregation` | Multi-GPU aggregation: `max`, `min`, `avg`, `sum`, `p95`, `p99` | `max` |
| `pollIntervalSeconds` | Metric polling interval | `10` |
| `vllmEndpoint` | vLLM engine metrics URL, e.g. `http://vllm-svc:8000/metrics`. Required when `metricType` is `vllm_queue_depth` or `vllm_kv_cache_usage` | (none) |
| `tritonEndpoint` | Triton engine metrics URL, e.g. `http://triton-svc:8002/metrics`. Required when `metricType` is `triton_queue_wait_ms` or `triton_request_rate` | (none) |

### Supported metricType values

| metricType | Unit | Description |
|------------|------|-------------|
| `gpu_utilization` | % | GPU compute utilization |
| `memory_utilization` | % | VRAM utilization reported by NVML |
| `memory_used_mib` | MiB | Raw VRAM usage |
| `memory_used_percent` | % | VRAM used as a percentage of total |
| `temperature` | °C | GPU die temperature |
| `power_draw` | W | GPU power consumption |
| `pcie_tx_kbps` | KB/s | PCIe transmit throughput (CPU→GPU) |
| `pcie_rx_kbps` | KB/s | PCIe receive throughput (GPU→CPU) |
| `nvlink_tx_mbps` | MB/s | Aggregate NVLink transmit throughput across all active links |
| `nvlink_rx_mbps` | MB/s | Aggregate NVLink receive throughput across all active links |
| `vllm_queue_depth` | count | Pending requests waiting in the vLLM engine (`vllm:num_requests_waiting`) — requires `vllmEndpoint`, see [vLLM Engine Metrics](#vllm-engine-metrics) |
| `vllm_kv_cache_usage` | % | vLLM GPU KV cache usage (`vllm:gpu_cache_usage_perc`, normalized to 0-100) — requires `vllmEndpoint`, see [vLLM Engine Metrics](#vllm-engine-metrics) |
| `triton_queue_wait_ms` | ms | Average Triton inference queue wait time, derived from `nv_inference_queue_duration_us` — requires `tritonEndpoint`, see [Triton Engine Metrics](#triton-engine-metrics) |
| `triton_request_rate` | requests/sec | Triton inference request rate, derived from `nv_inference_count` — requires `tritonEndpoint`, see [Triton Engine Metrics](#triton-engine-metrics) |

The `vllm_*` metrics bypass NVML entirely and are scraped directly from the
vLLM engine's own metrics endpoint. The `triton_*` metrics likewise bypass
NVML and are scraped from Triton's own metrics endpoint.

## Scaling Profiles

Profiles bundle defaults for common workloads. Override any parameter in the trigger metadata.

| Profile | Primary Metric | Target | Activation | Use Case |
|---------|---------------|--------|------------|----------|
| `vllm-inference` | Memory % | 80 | 5 | vLLM / LLM serving with scale-to-zero |
| `vllm-queue-depth` | Pending requests | 5 | 1 | vLLM — scale on queue depth via the engine API, see [vLLM Engine Metrics](#vllm-engine-metrics) |
| `triton-inference` | GPU Util | 75 | 10 | NVIDIA Triton Inference Server |
| `triton-queue-wait` | Queue wait (ms) | 50 | 5 | Triton — scale on average inference queue wait time via the engine API, see [Triton Engine Metrics](#triton-engine-metrics) |
| `triton-request-rate` | Requests/sec | 50 | 1 | Triton — scale on inference request rate via the engine API, see [Triton Engine Metrics](#triton-engine-metrics) |
| `training` | GPU Util | 90 | 0 | Training jobs (no scale-to-zero) |
| `batch` | Memory % | 70 | 1 | Batch inference with aggressive scale-down |
| `ollama` | Memory % | 70 | 3 | Ollama LLM serving with scale-to-zero |
| `tgi-inference` | Memory % | 75 | 5 | HuggingFace TGI serving with scale-to-zero |
| `distributed-training` | NVLink TX MB/s | 800 | 100 | Data-parallel training on NVLink systems |

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

## PCIe and NVLink Bandwidth Metrics

In data-parallel training (PyTorch DDP, DeepSpeed), GPUs constantly sync gradients via AllReduce. When communication bandwidth saturates, GPU compute utilization can appear low (40–60%) while the workload is actually fully bottlenecked. Standard GPU utilization metrics won't trigger scaling in this case — bandwidth metrics will.

### When to use PCIe metrics

Use `pcie_tx_kbps` / `pcie_rx_kbps` on nodes **without NVLink** (e.g. T4, A10, consumer-grade GPUs). On these systems all inter-GPU communication flows through the CPU over the PCIe bus (~32 GB/s). When PCIe saturates, adding replicas or reducing batch size helps more than waiting for GPU util to climb.

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "pcie_tx_kbps"
      targetValue: "28000000"     # ~28 GB/s (28,000,000 KB/s) — near PCIe Gen4 x16 limit
      activationThreshold: "1000000"
      aggregation: "max"
```

### When to use NVLink metrics

Use `nvlink_tx_mbps` / `nvlink_rx_mbps` on **NVSwitch / DGX / HGX** systems where GPUs communicate directly without the CPU (A100: ~600 GB/s aggregate, H100: ~900 GB/s). NVLink saturation indicates the model's communication pattern has outgrown the node — a signal to scale out or adjust parallelism strategy. The `distributed-training` profile uses NVLink TX; **tune the target to your hardware** — the built-in default targets ~50 GB/s which suits A100/H100 systems but may need adjustment for your link topology.

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "distributed-training"
      targetValue: "50000"        # MB/s (~50 GB/s) — tune to your hardware and link count
      activationThreshold: "5000"
```

### NVLink availability

On hardware without NVLink (T4, A10, etc.) the NVLink metrics are always `0`. If you configure a ScaledObject with an NVLink metric type on non-NVLink hardware, KEDA will see `0` and scale to zero if `activationThreshold > 0`. Use PCIe metrics on those nodes instead.

## vLLM Engine Metrics

GPU utilization and VRAM usage (the `vllm-inference` profile) are proxies for load — they tell you the GPU is busy, not how many requests are actually waiting. vLLM's own engine exposes that directly via its Prometheus `/metrics` endpoint, and `pkg/vllm` scrapes it so KEDA can scale on the real signal instead of waiting for a utilization or memory spike.

| metricType | Source metric | What it tells you |
|------------|----------------|--------------------|
| `vllm_queue_depth` | `vllm:num_requests_waiting` | Requests queued behind the running batch — the most direct signal for "we need more replicas now" |
| `vllm_kv_cache_usage` | `vllm:gpu_cache_usage_perc` | How full the KV cache is (0-100%) — a leading indicator before requests start queuing |

Both require `vllmEndpoint` — the full URL of the vLLM engine's metrics endpoint (e.g. `http://vllm-svc:8000/metrics`), reachable from the scaler DaemonSet pods. `vllmEndpoint` has nothing to do with NVML: it's a plain HTTP scrape of the inference server itself. `getMetricValue` routes any `vllm_*` metricType to this HTTP client instead of the NVML collector; the scaler keeps one cached client per distinct `vllmEndpoint`.

### Using the vllm-queue-depth profile

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "vllm-queue-depth"
      vllmEndpoint: "http://vllm-deepseek-deployment:8000/metrics"
```

### Using vllm_kv_cache_usage directly

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "vllm_kv_cache_usage"
      vllmEndpoint: "http://vllm-deepseek-deployment:8000/metrics"
      targetValue: "80"           # scale out once KV cache is 80% full
      activationThreshold: "5"
```

### vllm-inference vs. vllm-queue-depth

Use `vllm-inference` (VRAM-based) as a simple default — it needs no extra endpoint and scale-to-zero works out of the box. Switch to `vllm-queue-depth` (or raw `vllm_queue_depth` / `vllm_kv_cache_usage`) when you want faster reaction to load spikes than VRAM pressure alone provides, and can reach the vLLM engine's metrics port from the scaler's DaemonSet pods.

## Triton Engine Metrics

Like the vLLM engine metrics above, `triton-inference` (GPU utilization) is a proxy for load. NVIDIA Triton Inference Server exposes more direct signals on its own Prometheus `/metrics` endpoint (default port `8002`), and `pkg/triton` scrapes it so KEDA can scale on Triton's real request-handling behavior instead.

| metricType | Source metric | What it tells you |
|------------|----------------|--------------------|
| `triton_queue_wait_ms` | `nv_inference_queue_duration_us` | Average time inference requests spend waiting in Triton's scheduling queue — a direct sign the server can't keep up |
| `triton_request_rate` | `nv_inference_count` | Inference throughput (requests/sec) — useful for capacity-based scaling decisions |

Both require `tritonEndpoint` — the full URL of Triton's metrics endpoint (e.g. `http://triton-svc:8002/metrics`), reachable from the scaler DaemonSet pods. `getMetricValue` routes any `triton_*` metricType to this HTTP client instead of the NVML collector; the scaler keeps one cached client per distinct `tritonEndpoint`.

Unlike vLLM's queue depth (an instantaneous gauge), Triton reports `nv_inference_queue_duration_us` and `nv_inference_count` as **cumulative counters** that only ever increase. `pkg/triton` derives `triton_queue_wait_ms` and `triton_request_rate` by diffing two consecutive scrapes of the same endpoint — the change in cumulative queue time divided by the change in inference count for queue wait, and the change in inference count divided by elapsed time for request rate. This means:

- Both metrics report `0` on the very first scrape of a given `tritonEndpoint`, since there's no prior sample to diff against.
- They become meaningful after the scaler has polled the same endpoint at least twice, which happens naturally as KEDA (or `pollIntervalSeconds` in `StreamIsActive`) polls on an interval.
- If Triton restarts and its counters reset to a smaller value, the derived metrics fall back to `0` for that scrape rather than reporting a nonsensical negative rate.

### Using the triton-queue-wait profile

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      profile: "triton-queue-wait"
      tritonEndpoint: "http://triton-resnet-deployment:8002/metrics"
```

### Using triton_request_rate directly

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "triton_request_rate"
      tritonEndpoint: "http://triton-resnet-deployment:8002/metrics"
      targetValue: "100"          # scale out past 100 requests/sec
      activationThreshold: "1"
```

### triton-inference vs. triton-queue-wait / triton-request-rate

Use `triton-inference` (GPU-utilization-based) as a simple default — it needs no extra endpoint. Switch to `triton-queue-wait` or `triton-request-rate` when you want to scale on Triton's actual request-handling behavior rather than a GPU utilization proxy, and can reach Triton's metrics port from the scaler's DaemonSet pods.

## Multi-GPU Aggregation

On multi-GPU nodes, `aggregation` controls how per-GPU values are reduced to one number:

- **max** (default) — scale when any GPU hits the threshold. Good for inference where one hot GPU means overload.
- **avg** — scale on average utilization. Good for training where GPUs should be evenly loaded.
- **min** — scale when the least-loaded GPU hits the threshold. Conservative.
- **sum** — total utilization. Useful for capacity-based decisions.
- **p95** / **p99** — percentile of per-GPU values (nearest-rank: values are sorted and the element at the percentile's rank is used). On nodes with several GPUs, a single hot GPU can dominate `max` and trigger unnecessary scaling; percentile aggregation lets you ignore that kind of outlier while still reacting to broad load. With few GPUs (roughly 8 or fewer), `p95`/`p99` will often equal `max`, since there aren't enough samples to leave an outlier out of the selected rank.

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
| `keda_gpu_scaler_gpu_pcie_throughput_kbps` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name`, `direction` | PCIe throughput in KB/s — `direction`: `tx` or `rx` |
| `keda_gpu_scaler_gpu_nvlink_throughput_mbps` | Gauge | `gpu_index`, `gpu_uuid`, `gpu_name`, `direction` | Aggregate NVLink throughput in MB/s — `direction`: `tx` or `rx` |
| `keda_gpu_scaler_gpu_device_count` | Gauge | — | Number of GPU devices detected on this node |
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

Check `deploy/examples/` for ScaledObject manifests:

- `vllm-scaledobject.yaml` — vLLM inference with scale-to-zero
- `vllm-queue-depth-scaledobject.yaml` — vLLM queue depth scaling via the engine API
- `triton-queue-wait-scaledobject.yaml` — Triton queue wait time scaling via the engine API
- `triton-request-rate-scaledobject.yaml` — Triton request rate scaling via the engine API
- `custom-gpu-utilization.yaml` — raw GPU utilization scaling
