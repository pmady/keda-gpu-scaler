#!/usr/bin/env bash
#
# Tear down the local GPU test cluster created by setup.sh.
#
# Usage:
#   ./teardown.sh           # remove the demo + Helm releases, keep the cluster
#   ./teardown.sh --all     # also delete the whole minikube cluster
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KEDA_NAMESPACE="${KEDA_NAMESPACE:-keda}"
PROFILE="${MINIKUBE_PROFILE:-minikube}"

log() { printf '\n\033[1;36m==> %s\033[0m\n' "$*"; }

log "Removing demo resources"
# Tolerate failures: if setup never completed, the KEDA CRDs (and thus the
# ScaledObject kind) may not exist, which makes `kubectl delete` error out
# rather than no-op. Don't let that abort the rest of the teardown.
kubectl delete -f "$SCRIPT_DIR/demo/gpu-load.yaml" --ignore-not-found 2>/dev/null || true
kubectl delete -f "$SCRIPT_DIR/demo/scaledobject.yaml" --ignore-not-found 2>/dev/null || true
kubectl delete -f "$SCRIPT_DIR/demo/scale-target.yaml" --ignore-not-found 2>/dev/null || true

log "Uninstalling Helm releases"
helm uninstall keda-gpu-scaler -n "$KEDA_NAMESPACE" --ignore-not-found || true
helm uninstall keda -n "$KEDA_NAMESPACE" --ignore-not-found || true

if [[ "${1:-}" == "--all" ]]; then
  log "Deleting the minikube cluster (profile: $PROFILE)"
  minikube -p "$PROFILE" delete
else
  echo
  echo "Helm releases and demo removed. The minikube cluster is still running."
  echo "Run './teardown.sh --all' to delete the cluster entirely."
fi
