# e2e scale target + ScaledObject, installed last so it's destroyed FIRST —
# while KEDA is still running to process the ScaledObject's finalizer.
resource "helm_release" "e2e" {
  name      = "keda-gpu-scaler-e2e"
  namespace = "default"

  chart = "${path.module}/../../../deploy/helm/keda-gpu-scaler-e2e"

  set = [
    {
      name  = "scalerAddress"
      value = "${var.scaler_release_name}.${var.keda_namespace}.svc.cluster.local:6000"
    },
  ]

  wait    = true
  timeout = var.helm_timeout

  depends_on = [helm_release.keda_gpu_scaler]
}