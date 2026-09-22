# Benchmarks

Performance characteristics of keda-gpu-scaler.

## Metric Latency

Time from GPU state change to KEDA receiving the updated metric.

| Approach | Latency | Components |
|----------|---------|------------|
| dcgm-exporter → Prometheus → KEDA | 15-30s | 5 |
| **keda-gpu-scaler (direct NVML)** | **KEDA `pollingInterval` + ~ms** | **2** |

The scaler reads NVML at the moment KEDA calls `GetMetrics`, so there is no scrape interval or adapter delay in the path. Metric age is the `pollingInterval` you set on the ScaledObject plus a single NVML read (see below); with `pollingInterval: 2` that is a few seconds.

## NVML Poll Overhead

Single NVML call to read all metrics for one GPU:

| Operation | Time |
|-----------|------|
| `nvmlDeviceGetUtilizationRates` | ~0.1ms |
| `nvmlDeviceGetMemoryInfo` | ~0.1ms |
| `nvmlDeviceGetTemperature` | ~0.05ms |
| `nvmlDeviceGetPowerUsage` | ~0.05ms |
| **Total per GPU** | **~0.3ms** |

For an 8-GPU node, full metric collection takes ~2.5ms.

## Memory Usage

| GPUs per Node | Memory |
|---------------|--------|
| 1 | ~18Mi |
| 4 | ~20Mi |
| 8 | ~22Mi |

Memory is dominated by the Go runtime and gRPC server, not metric storage.

## gRPC Response Time

Time to respond to KEDA's `GetMetrics` call (metrics already cached):

| Metric | p50 | p99 |
|--------|-----|-----|
| Single GPU | 0.2ms | 0.8ms |
| 8 GPU (max aggregation) | 0.3ms | 1.2ms |

## Scale-to-Zero Activation

Time from first request hitting an idle deployment to pod becoming ready:

| Component | Time |
|-----------|------|
| KEDA polling interval | 2s (configurable) |
| `IsActive` check | <1ms |
| HPA scale decision | ~1s |
| Pod scheduling + startup | workload-dependent |

Total activation latency is dominated by pod startup time, not the scaler.

## Production Observations

From a 4-node A100 80GB cluster running vLLM inference:

- Scaling decisions triggered within 3-4 seconds of load spike
- No missed scale events over 30-day observation period
- Zero impact on inference latency from metric collection
- Scale-to-zero working reliably with 60s cooldown

## Running Your Own Benchmarks

```bash
# Build with race detector disabled for accurate timing
CGO_ENABLED=1 go build -o keda-gpu-scaler ./cmd/keda-gpu-scaler/

# Run with debug logging to see metric collection timing
./keda-gpu-scaler --log-level=debug
```

For gRPC latency testing, use `grpcurl`:

```bash
grpcurl -plaintext localhost:6000 externalscaler.ExternalScaler/GetMetrics
```
