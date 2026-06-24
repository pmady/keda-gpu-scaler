# Local GPU test cluster (minikube)

Run KEDA + `keda-gpu-scaler` against the **real NVIDIA GPU in your own machine**,
for free, in a local single-node Kubernetes cluster. This is the local
counterpart to [`infra/terraform/aws`](../terraform/aws) — same end result (a
cluster that scales a workload based on GPU metrics), but on your hardware
instead of rented cloud GPUs.

On **Windows**, run all of this **inside WSL2** (Ubuntu). The GPU is shared into
WSL2 automatically by the Windows NVIDIA driver.

> [!NOTE]
> Local GPU-in-Kubernetes has sharp edges (passing a GPU into a containerized
> cluster is inherently fiddly). The scripts here automate the happy path and
> the README's Troubleshooting section covers the common snags. If something
> breaks, the error is almost always one of: GPU not visible to Docker, or the
> NVML library path inside the node.

## What you'll end up with

```
your machine (NVIDIA GPU)
└─ WSL2 / Linux
   └─ Docker (with NVIDIA Container Toolkit)
      └─ minikube node (gets the GPU via --gpus all)
         ├─ NVIDIA device plugin   (exposes nvidia.com/gpu)
         ├─ KEDA                    (the autoscaler engine)
         └─ keda-gpu-scaler         (reads your GPU, built from this repo)
```

## Prerequisites

1. **An NVIDIA GPU visible in WSL2.** In your Ubuntu/WSL2 terminal:
   ```bash
   nvidia-smi
   ```
   Must print a table with your GPU. If not, update your Windows NVIDIA driver
   (the driver lives on Windows; you do **not** install one inside WSL2).

2. **Docker** running in WSL2 (Docker Desktop with WSL integration, or Docker
   Engine installed directly in Ubuntu).

3. **NVIDIA Container Toolkit** (lets Docker run GPU containers). On Ubuntu/WSL2:
   ```bash
   curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey \
     | sudo gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg
   curl -s -L https://nvidia.github.io/libnvidia-container/stable/deb/nvidia-container-toolkit.list \
     | sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' \
     | sudo tee /etc/apt/sources.list.d/nvidia-container-toolkit.list
   sudo apt update && sudo apt install -y nvidia-container-toolkit
   sudo nvidia-ctk runtime configure --runtime=docker
   sudo systemctl restart docker   # or restart Docker Desktop
   ```
   Verify Docker can see the GPU:
   ```bash
   docker run --rm --gpus all nvidia/cuda:12.4.1-base-ubuntu22.04 nvidia-smi
   ```

4. **minikube, kubectl, helm**:
   ```bash
   # minikube
   curl -fsSLo minikube https://storage.googleapis.com/minikube/releases/latest/minikube-linux-amd64
   sudo install minikube /usr/local/bin/ && rm minikube
   # kubectl
   curl -fsSLO "https://dl.k8s.io/release/$(curl -fsSL https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
   sudo install kubectl /usr/local/bin/ && rm kubectl
   # helm
   curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
   ```

## Quick start

From this directory (`infra/local`) inside WSL2:

```bash
./setup.sh
```

That single command:
1. checks your tools and that Docker can see the GPU,
2. starts minikube with the GPU passed in and enables the NVIDIA device plugin,
3. builds the scaler image from this repo straight into minikube's Docker,
4. installs KEDA, then
5. installs `keda-gpu-scaler` from the in-tree Helm chart with local-friendly
   settings ([`values-scaler-local.yaml`](./values-scaler-local.yaml)).

Then confirm the scaler is up and actually reading your GPU:

```bash
kubectl -n keda get pods
kubectl -n keda logs ds/keda-gpu-scaler --tail=20
```

You should see it log GPU metrics rather than an NVML error.

## Run the demo

This shows KEDA scaling a dummy `demo-app` Deployment up when the GPU is busy and
back down when it's idle. Use two or three terminals.

```bash
# 1. Deploy the workload and the scaling rule
kubectl apply -f demo/scale-target.yaml
kubectl apply -f demo/scaledobject.yaml

# 2. Watch the replica count (leave this running)
kubectl get deploy demo-app -w

# 3. In another terminal, drive the GPU to 100% for ~2.5 minutes
kubectl apply -f demo/gpu-load.yaml
```

Within a polling interval or two you should see `demo-app` climb from 1 replica
toward 5 as GPU utilization crosses the target, then fall back to 1 after the
`gpu-burn` job finishes and the GPU goes idle. That's the full loop:
**scaler reads the GPU → KEDA acts on it → your workload scales.**

Useful while watching:
```bash
kubectl get scaledobject demo-app-gpu-scaler   # READY/ACTIVE columns
kubectl get hpa                                 # KEDA drives an HPA under the hood
kubectl -n keda logs ds/keda-gpu-scaler -f      # live metrics from the scaler
```

## Teardown

```bash
./teardown.sh          # remove the demo + KEDA + scaler, keep the cluster
./teardown.sh --all    # also delete the whole minikube cluster
```

## Troubleshooting

**Scaler pod is `CrashLoopBackOff` with "NVML init failed" / can't open
`libnvidia-ml.so`.**
The library path host-mounted into the pod is wrong for your node. Find the real
path inside the minikube node and update `nvmlLib` in
`values-scaler-local.yaml`, then re-run `./setup.sh`:
```bash
minikube ssh -- 'find / -name "libnvidia-ml.so*" 2>/dev/null'
```

**`docker run --gpus all ... nvidia-smi` fails.**
The NVIDIA Container Toolkit isn't installed/configured (prerequisite 3), or
Docker wasn't restarted afterwards. Fix that before running `setup.sh`.

**Node never advertises `nvidia.com/gpu`.**
Check the device plugin: `kubectl -n kube-system get pods | grep nvidia` and
`kubectl describe node | grep -A5 Allocatable`. Make sure minikube started with
`--gpus=all` (re-run `minikube delete` then `./setup.sh` if you started it
without GPU support earlier).

**`demo-app` doesn't scale up.**
Confirm the GPU is actually loaded (`kubectl -n keda logs ds/keda-gpu-scaler -f`
should show rising utilization while `gpu-burn` runs), and that the ScaledObject
is `READY=True` (`kubectl get scaledobject`). The `gpu-burn` image must have
pulled and be `Running` (`kubectl get pods -l app=gpu-burn`).

**The `gpu-burn` image won't pull.**
Swap `image`/`args` in `demo/gpu-load.yaml` for any GPU workload you have — the
scaler measures utilization globally, so anything that keeps the GPU busy works.
