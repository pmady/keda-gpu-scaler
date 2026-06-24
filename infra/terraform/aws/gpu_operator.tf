# NVIDIA GPU operator.
#
# The AL2023 NVIDIA AMI (var.gpu_ami_type default) already ships the host
# driver, CUDA user-mode driver and the NVIDIA container toolkit, and
# containerd is preconfigured with the `nvidia` runtime. So we disable the
# operator's driver and toolkit components and let it provide only what the AMI
# does not:
#   - the NVIDIA k8s device plugin (advertises nvidia.com/gpu),
#   - node-feature-discovery + GPU-feature-discovery, which apply the
#     `nvidia.com/gpu.present=true` node label the scaler's nodeSelector
#     targets,
#   - DCGM / dcgm-exporter,
#   - the `nvidia` RuntimeClass referenced by the scaler pod template.
#
# If you switch to a non-NVIDIA AMI (e.g. plain AL2023), set driver.enabled=true
# so the operator installs the driver itself.
resource "helm_release" "gpu_operator" {
  name             = "gpu-operator"
  namespace        = "gpu-operator"
  create_namespace = true

  repository = "https://helm.ngc.nvidia.com/nvidia"
  chart      = "gpu-operator"
  version    = var.gpu_operator_chart_version

  set = [
    {
      name  = "driver.enabled"
      value = "false"
    },
    {
      name  = "toolkit.enabled"
      value = "false"
    },
  ]

  # Device-plugin/GFD rollout and node labelling can take a few minutes after
  # the node joins.
  wait    = true
  timeout = var.helm_timeout

  depends_on = [module.eks]
}
