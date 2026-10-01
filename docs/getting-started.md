# Getting Started

## Prerequisites

- Kubernetes cluster with NVIDIA GPU worker nodes (OKE, GKE, EKS, AKS, or bare metal)
- [KEDA v2.10+](https://keda.sh/docs/latest/deploy/) installed
- NVIDIA GPU drivers and [Device Plugin](https://github.com/NVIDIA/k8s-device-plugin) installed

> **Note:** The scaler links NVML and loads `libnvidia-ml.so` at runtime. The binary (and the container image) will not start on a host that lacks the NVIDIA driver. On GPU nodes the driver supplies this library; for local development without a GPU, use the mock collector exercised by the test suite instead of running the binary directly.

## Deploy the Scaler

Make sure KEDA is already running, then deploy:

```bash
kubectl apply -f deploy/manifests.yaml
```

Or use Helm:

```bash
helm install keda-gpu-scaler deploy/helm/keda-gpu-scaler \
  --namespace keda
```

This puts a pod on every GPU node, polling NVML and serving metrics over gRPC on port 6000.

The chart schedules pods onto nodes labelled `nvidia.com/gpu.present=true` and
uses the `nvidia` RuntimeClass. Both are created by the NVIDIA GPU Operator. On
a cluster without the operator, label the GPU nodes yourself and mount the
driver from the host instead:

```bash
kubectl label node <gpu-node> nvidia.com/gpu.present=true

helm install keda-gpu-scaler deploy/helm/keda-gpu-scaler \
  --namespace keda \
  --set runtimeClassName="" \
  --set nvmlHostMounts.enabled=true
```

If you override `nodeSelector` on the command line, use `--set-string` so
Helm keeps the label value as a string; Kubernetes rejects `true` as a
boolean.

If KEDA is not installed yet, `helm upgrade --install keda kedacore/keda
--namespace keda --create-namespace` installs it, and is safe to rerun if it
already is. The same `upgrade --install` form works for the scaler itself and
is what you want in scripts, since plain `helm install` fails when the release
already exists.

## Try it without a GPU

On a cluster with no GPU hardware (a kind or k3d cluster, a lab playground),
the scaler can serve synthetic GPU metrics so you can see KEDA scale a
workload before you have real devices. The scaler reads nothing from the
host in this mode, so do not use it on GPU nodes.

```bash
helm upgrade --install keda-gpu-scaler deploy/helm/keda-gpu-scaler \
  --namespace keda --create-namespace \
  --set mock.enabled=true \
  --set mock.utilization=85 \
  --set nodeSelector=null \
  --set runtimeClassName="" \
  --set tolerations=null
```

Every node then reports one GPU at 85% utilization. The repository ships a
demo target and ScaledObject in `deploy/helm/keda-gpu-scaler-e2e`:

```bash
helm upgrade --install gpu-demo deploy/helm/keda-gpu-scaler-e2e --set minReplicaCount=0
kubectl get hpa,scaledobject
```

With a target of 30% the HPA settles at ceil(85 / 30) = 3 replicas. Changing
`mock.utilization` rolls out a new scaler pod, so allow for the DaemonSet
rollout plus KEDA's 15 second poll and the 30 second cooldown before expecting
a change:

```bash
helm upgrade keda-gpu-scaler deploy/helm/keda-gpu-scaler -n keda --reuse-values --set mock.utilization=0
kubectl rollout status ds/keda-gpu-scaler -n keda && sleep 120
kubectl get deploy/demo-app
helm upgrade keda-gpu-scaler deploy/helm/keda-gpu-scaler -n keda --reuse-values --set mock.utilization=85
kubectl rollout status ds/keda-gpu-scaler -n keda && kubectl get deploy/demo-app -w
```

The event log then shows the whole loop, including the step out of zero that
the scaler decides on its own through `IsActive`:

```
KEDAScaleTargetDeactivated   Deactivated apps/v1.Deployment default/demo-app from 3 to 0
ScalingReplicaSet            Scaled down replica set demo-app-7cf655cb87 from 3 to 0
KEDAScaleTargetActivated     Scaled apps/v1.Deployment default/demo-app from 0 to 1, triggered by s0-keda_gpu_metric
SuccessfulRescale            New size: 3; reason: external metric s0-keda_gpu_metric above target
```

Scaling back down to a non-zero minimum takes up to five minutes because of
the HPA's default downscale stabilization window. The 30 second
`cooldownPeriod` only applies to the step to zero.

## Attach to Your Workload

Point a ScaledObject at the GPU scaler:

```yaml
apiVersion: keda.sh/v1alpha1
kind: ScaledObject
metadata:
  name: vllm-inference-scaler
  namespace: ai-workloads
spec:
  scaleTargetRef:
    name: vllm-deepseek-deployment
  minReplicaCount: 1
  maxReplicaCount: 50
  triggers:
    - type: external
      metadata:
        scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
        profile: "vllm-inference"
```

KEDA will now scale your deployment based on GPU utilization and VRAM pressure.

## Verify It Works

```bash
# Check scaler pods are running on GPU nodes
kubectl get pods -n keda -l app=keda-gpu-scaler -o wide

# Check KEDA sees the ScaledObject
kubectl get scaledobject -A

# Watch HPA in action
kubectl get hpa -w
```

## What to read next

- [Configuration Reference](configuration.md) for profiles, aggregation, and all parameters
- [Architecture](DESIGN.md) if you want to understand the design
- [Migration Guide](MIGRATION.md) if you're replacing dcgm-exporter + Prometheus
