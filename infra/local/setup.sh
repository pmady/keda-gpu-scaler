#!/usr/bin/env bash
#
# Stand up a local single-node GPU Kubernetes cluster (minikube) and install
# KEDA + keda-gpu-scaler on it, using your real NVIDIA GPU. Intended for WSL2 on
# Windows (or any Linux box with an NVIDIA GPU + Docker + the NVIDIA Container
# Toolkit).
#
# This is the local counterpart to infra/terraform/aws. Re-running it is safe;
# each step is idempotent. See README.md for prerequisites and troubleshooting.
#
# Usage:  ./setup.sh
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

KEDA_CHART_VERSION="${KEDA_CHART_VERSION:-2.20.1}"
KEDA_NAMESPACE="${KEDA_NAMESPACE:-keda}"
SCALER_IMAGE="${SCALER_IMAGE:-keda-gpu-scaler:dev}"
PROFILE="${MINIKUBE_PROFILE:-minikube}"

log()  { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }
die()  { printf '\n\033[1;31mERROR: %s\033[0m\n' "$*" >&2; exit 1; }

# --- 0. Pre-flight -----------------------------------------------------------
log "Checking required tools"
for tool in nvidia-smi docker minikube kubectl helm; do
  command -v "$tool" >/dev/null 2>&1 || die "'$tool' not found on PATH. See README.md prerequisites."
done

log "Checking the GPU is visible to Docker"
if ! docker run --rm --gpus all nvidia/cuda:12.4.1-base-ubuntu22.04 nvidia-smi >/dev/null 2>&1; then
  die "Docker can't see the GPU ('docker run --gpus all ... nvidia-smi' failed).
Install/configure the NVIDIA Container Toolkit in WSL2 first — see README.md."
fi

# --- 1. Cluster --------------------------------------------------------------
log "Starting minikube with GPU support (profile: $PROFILE)"
if minikube -p "$PROFILE" status >/dev/null 2>&1; then
  echo "minikube profile '$PROFILE' already running — leaving it as is."
else
  minikube start -p "$PROFILE" \
    --driver=docker \
    --container-runtime=docker \
    --gpus=all
fi

log "Enabling the NVIDIA device plugin addon"
minikube -p "$PROFILE" addons enable nvidia-device-plugin

log "Waiting for the node to advertise a GPU (nvidia.com/gpu)"
for _ in $(seq 1 30); do
  if kubectl get nodes -o jsonpath='{.items[*].status.allocatable.nvidia\.com/gpu}' 2>/dev/null | grep -qE '[1-9]'; then
    echo "GPU is allocatable."
    break
  fi
  sleep 5
done
kubectl get nodes -o jsonpath='{.items[*].status.allocatable.nvidia\.com/gpu}' 2>/dev/null | grep -qE '[1-9]' \
  || die "Node never advertised nvidia.com/gpu. Check 'kubectl describe node' and README troubleshooting."

# --- 2. Build the scaler image into minikube's Docker ------------------------
log "Building the keda-gpu-scaler image ($SCALER_IMAGE) inside minikube's Docker"
# Point docker at minikube's daemon so the built image is available to the
# cluster without pushing to a registry.
eval "$(minikube -p "$PROFILE" docker-env)"
docker build -t "$SCALER_IMAGE" "$REPO_ROOT"
# Switch docker back to the host daemon.
eval "$(minikube -p "$PROFILE" docker-env -u)"

# --- 3. KEDA -----------------------------------------------------------------
log "Installing KEDA $KEDA_CHART_VERSION (Helm)"
helm repo add kedacore https://kedacore.github.io/charts >/dev/null 2>&1 || true
helm repo update kedacore >/dev/null
helm upgrade --install keda kedacore/keda \
  --namespace "$KEDA_NAMESPACE" --create-namespace \
  --version "$KEDA_CHART_VERSION" \
  --wait --timeout 5m

# --- 4. keda-gpu-scaler (from the in-tree chart) -----------------------------
log "Installing keda-gpu-scaler from the in-tree chart"
helm upgrade --install keda-gpu-scaler "$REPO_ROOT/deploy/helm/keda-gpu-scaler" \
  --namespace "$KEDA_NAMESPACE" \
  -f "$SCRIPT_DIR/values-scaler-local.yaml" \
  --wait --timeout 5m

# --- Done --------------------------------------------------------------------
log "Done. Cluster is ready."
cat <<EOF

The scaler is running in the '$KEDA_NAMESPACE' namespace. Verify it sees your GPU:

  kubectl -n $KEDA_NAMESPACE get pods
  kubectl -n $KEDA_NAMESPACE logs ds/keda-gpu-scaler --tail=20

Run the autoscaling demo (in three terminals — see README.md "Run the demo"):

  kubectl apply -f $SCRIPT_DIR/demo/scale-target.yaml
  kubectl apply -f $SCRIPT_DIR/demo/scaledobject.yaml
  watch kubectl get deploy demo-app          # watch replicas
  kubectl apply -f $SCRIPT_DIR/demo/gpu-load.yaml   # drive the GPU -> scale up

Tear everything down with:

  ./teardown.sh
EOF
