# KEDA. Installed because the turnkey goal is a cluster that's ready for
# integration tests with no manual steps; the scaler's chart also renders a
# KEDA ScaledObject, so the KEDA CRDs must exist before the scaler is installed.
resource "helm_release" "keda" {
  name             = "keda"
  namespace        = var.keda_namespace
  create_namespace = true

  repository = "https://kedacore.github.io/charts"
  chart      = "keda"
  version    = var.keda_chart_version

  wait    = true
  timeout = var.helm_timeout

  depends_on = [azurerm_kubernetes_cluster.this]
}

locals {
  # Only override chart values that were explicitly set; otherwise fall back to
  # the in-tree chart defaults (nodeSelector nvidia.com/gpu.present=true,
  # runtimeClassName: nvidia, image ghcr.io/pmady/keda-gpu-scaler:<appVersion>).
  scaler_set = concat(
    var.scaler_image_repository != "" ? [{ name = "image.repository", value = var.scaler_image_repository }] : [],
    var.scaler_image_tag != "" ? [{ name = "image.tag", value = var.scaler_image_tag }] : [],
    var.scaler_runtime_class_name != null ? [{ name = "runtimeClassName", value = var.scaler_runtime_class_name }] : [],
  )
}

# keda-gpu-scaler, installed FROM the in-tree Helm chart so the test cluster
# always runs the local version of the scaler rather than a published release.
resource "helm_release" "keda_gpu_scaler" {
  name      = var.scaler_release_name
  namespace = var.keda_namespace

  chart = "${path.module}/../../../deploy/helm/keda-gpu-scaler"

  set = local.scaler_set

  wait    = true
  timeout = var.helm_timeout

  # The chart renders a KEDA ScaledObject (needs the KEDA CRDs), and its
  # DaemonSet pod selects the `nvidia.com/gpu.present` label and the `nvidia`
  # RuntimeClass that the GPU operator provisions — so both must be in place
  # first or the pod stays Pending and the release times out.
  depends_on = [
    helm_release.keda,
    helm_release.gpu_operator,
  ]
}
