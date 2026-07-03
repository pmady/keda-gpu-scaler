# NVIDIA GPU operator.
#
# The GPU node pool is created with gpu_driver = "None" (see main.tf), so AKS
# installs NO GPU software. The operator therefore owns the entire stack — the
# opposite split from the EKS sibling, where the AL2023 NVIDIA AMI already ships
# the driver/toolkit so the operator only adds the device plugin. Here we let
# the operator provide:
#   - the NVIDIA host driver (driver.enabled = true),
#   - the NVIDIA container toolkit, which configures containerd's `nvidia`
#     runtime handler the `nvidia` RuntimeClass points at (toolkit.enabled = true),
#   - the NVIDIA k8s device plugin (advertises nvidia.com/gpu),
#   - node-feature-discovery + GPU-feature-discovery, which apply the
#     `nvidia.com/gpu.present=true` node label the scaler's nodeSelector targets,
#   - DCGM / dcgm-exporter,
#   - the `nvidia` RuntimeClass referenced by the scaler pod template.
#
# This is NVIDIA's documented approach for AKS (skip the AKS driver, run the
# operator): https://learn.microsoft.com/azure/aks/nvidia-gpu-operator
resource "helm_release" "gpu_operator" {
  name             = "gpu-operator"
  namespace        = "gpu-operator"
  create_namespace = true

  repository = "https://helm.ngc.nvidia.com/nvidia"
  chart      = "gpu-operator"
  version    = var.gpu_operator_chart_version

  # Both are the operator defaults; set explicitly to document that on AKS the
  # operator — not the node image — installs the driver and toolkit.
  set = [
    {
      name  = "driver.enabled"
      value = "true"
    },
    {
      name  = "toolkit.enabled"
      value = "true"
    },
  ]

  # Driver build + device-plugin/GFD rollout and node labelling can take several
  # minutes after the node joins.
  wait    = true
  timeout = var.helm_timeout

  depends_on = [azurerm_kubernetes_cluster.this]
}
