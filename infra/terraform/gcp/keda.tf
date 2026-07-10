# KEDA CRDs must exist before the scaler chart (which renders a ScaledObject) installs.
resource "helm_release" "keda" {
  name             = "keda"
  namespace        = var.keda_namespace
  create_namespace = true

  repository = "https://kedacore.github.io/charts"
  chart      = "keda"
  version    = var.keda_chart_version

  wait    = true
  timeout = var.helm_timeout

  depends_on = [google_container_cluster.primary, google_container_node_pool.gpu]
}

locals {
  # Only override chart values explicitly set; otherwise fall back to chart defaults.
  scaler_set = concat(
    var.scaler_image_repository != "" ? [{ name = "image.repository", value = var.scaler_image_repository }] : [],
    var.scaler_image_tag != "" ? [{ name = "image.tag", value = var.scaler_image_tag }] : [],
    var.scaler_runtime_class_name != null ? [{ name = "runtimeClassName", value = var.scaler_runtime_class_name }] : [],
  )
}

# Installed from the in-tree chart so the cluster runs the local scaler build.
resource "helm_release" "keda_gpu_scaler" {
  name      = var.scaler_release_name
  namespace = var.keda_namespace

  chart = "${path.module}/../../../deploy/helm/keda-gpu-scaler"

  set = local.scaler_set

  # wait=true: the operator (also wait=true) has already created the nvidia.com/gpu.present
  # label + nvidia RuntimeClass by the time this installs, so there's no async race.
  wait = true

  # Bounds the `helm uninstall` on destroy.
  timeout = var.helm_timeout

  # Chart renders a KEDA ScaledObject, so KEDA + the operator must apply first.
  depends_on = [
    helm_release.keda,
    helm_release.gpu_operator,
  ]
}
