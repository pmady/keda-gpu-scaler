# Migrating from dcgm-exporter + Prometheus to keda-gpu-scaler

Replace the dcgm-exporter → Prometheus → KEDA Prometheus scaler pipeline with keda-gpu-scaler.

## What You're Replacing

```
BEFORE:
  GPU Pod → dcgm-exporter (DaemonSet) → Prometheus (StatefulSet) → PromQL → KEDA Prometheus trigger → HPA

AFTER:
  GPU Pod → keda-gpu-scaler (DaemonSet) → KEDA External trigger → HPA
```

You're removing 3 components from the scaling path: dcgm-exporter, Prometheus, and the PromQL query layer.

## Step 1: Deploy keda-gpu-scaler

Keep your existing dcgm-exporter + Prometheus pipeline running during migration. Deploy keda-gpu-scaler alongside it:

```bash
# Helm
helm install keda-gpu-scaler deploy/helm/keda-gpu-scaler \
  --namespace keda

# Or manifests
kubectl apply -f deploy/manifests.yaml
```

Verify it's running on your GPU nodes:

```bash
kubectl get pods -n keda -l app=keda-gpu-scaler -o wide
```

## Step 2: Update Your ScaledObject

Replace the Prometheus trigger with an external trigger.

### Before (Prometheus trigger)

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: vllm-scaler
spec:
  scaleTargetRef:
    name: vllm-deployment
  minReplicaCount: 1
  maxReplicaCount: 10
  triggers:
    - type: prometheus
      metadata:
        serverAddress: http://prometheus.monitoring.svc:9090
        metricName: DCGM_FI_DEV_GPU_UTIL
        query: avg(DCGM_FI_DEV_GPU_UTIL{pod=~"vllm-.*"})
        threshold: "80"
```

### After (external trigger)

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: vllm-scaler
spec:
  scaleTargetRef:
    name: vllm-deployment
  minReplicaCount: 1
  maxReplicaCount: 10
  triggers:
    - type: external
      metadata:
        scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
        profile: "vllm-inference"
```

Or with explicit metric configuration:

```yaml
triggers:
  - type: external
    metadata:
      scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
      metricType: "gpu_utilization"
      targetValue: "80"
      activationThreshold: "5"
```

## Step 3: Metric Mapping

DCGM metric → keda-gpu-scaler equivalent:

| DCGM Metric (Prometheus) | keda-gpu-scaler equivalent | Notes |
|--------------------------|---------------------------|-------|
| `DCGM_FI_DEV_GPU_UTIL` | `gpu_utilization` | SM utilization % |
| `DCGM_FI_DEV_MEM_COPY_UTIL` | `memory_utilization` | Memory controller % |
| `DCGM_FI_DEV_FB_USED` | `memory_used_mib` | Frame buffer used in MiB |
| `DCGM_FI_DEV_GPU_TEMP` | `temperature` | GPU die temp in Celsius |
| `DCGM_FI_DEV_POWER_USAGE` | `power_draw` | Power in Watts |
| `DCGM_FI_DEV_FB_USED / DCGM_FI_DEV_FB_TOTAL` | `memory_used_percent` | Computed by scaler |

## Step 4: Verify Scaling Behavior

Watch the HPA and scaler logs side by side:

```bash
# Scaler logs
kubectl logs -n keda -l app=keda-gpu-scaler -f

# HPA status
kubectl get hpa -w

# ScaledObject status
kubectl get scaledobject vllm-scaler -o yaml | grep -A10 status
```

Run a load test against your inference endpoint and confirm replicas scale up when GPU utilization rises.

## Step 5: Remove dcgm-exporter (Optional)

Once you've confirmed scaling works through keda-gpu-scaler, you can remove dcgm-exporter from the scaling path.

If you still need dcgm-exporter for Grafana dashboards or monitoring (not scaling), keep it running — it doesn't conflict with keda-gpu-scaler. They both read NVML independently.

If you only had dcgm-exporter for KEDA scaling:

```bash
# Remove dcgm-exporter DaemonSet
kubectl delete daemonset dcgm-exporter -n gpu-operator

# Remove the Prometheus scrape config for dcgm metrics (if dedicated)
# Remove the KEDA TriggerAuthentication for Prometheus (if used)
```

## What You Gain

| | dcgm-exporter + Prometheus | keda-gpu-scaler |
|---|---|---|
| Metric latency | 15-30s (scrape interval) | Sub-second |
| Components in scaling path | 5 | 2 |
| Configuration | PromQL query per workload | 3-line trigger or profile name |
| Failure mode | Prometheus outage = no scaling | Node-local, no single point of failure |
| Maintenance | PromQL queries break on DCGM version upgrades | No queries to maintain |

## Rollback

If you need to revert, just switch the ScaledObject trigger back to `type: prometheus` and delete the keda-gpu-scaler DaemonSet. No data migration required.
