locals {
  tags = merge(
    {
      Project   = "keda-gpu-scaler"
      Component = "gpu-integration-test"
      ManagedBy = "terraform"
      Stack     = "infra/terraform/azure"
    },
    var.tags,
  )
}

###############################################################################
# Resource group
###############################################################################

resource "azurerm_resource_group" "this" {
  name     = var.resource_group_name
  location = var.location
  tags     = local.tags
}

###############################################################################
# AKS control plane + single GPU node pool
#
# Hand-rolled on the azurerm `azurerm_kubernetes_cluster` resource rather than
# the Azure/aks/azurerm community module (evaluated at v11.7.0). Two reasons:
#   1. AKS — unlike EKS — manages its own VNet, so there is no networking module
#      to lean on; the native resource IS the minimal path.
#   2. The module only exposes `gpu_driver` on extra `node_pools`, not on the
#      system/default pool, which would force a 2-pool (system + GPU) design.
#      Here we make the *default* pool the GPU pool with `gpu_driver = "None"`,
#      so a single untainted node runs the whole stack (operator, KEDA, CoreDNS
#      and the scaler) — the cheapest layout and a direct mirror of the EKS
#      sibling's single untainted GPU node group.
###############################################################################

resource "azurerm_kubernetes_cluster" "this" {
  name                = var.cluster_name
  location            = azurerm_resource_group.this.location
  resource_group_name = azurerm_resource_group.this.name
  dns_prefix          = var.cluster_name
  kubernetes_version  = var.kubernetes_version

  default_node_pool {
    name       = "gpu"
    vm_size    = var.gpu_vm_size
    node_count = var.gpu_node_count

    # Fixed-size pool: predictable for tests, no autoscaling, no spot.
    auto_scaling_enabled = false

    os_disk_size_gb = var.gpu_node_disk_size

    # Skip AKS's built-in NVIDIA driver so the GPU operator owns the full stack
    # (driver + container toolkit + device plugin + GFD label + `nvidia`
    # RuntimeClass). This is NVIDIA's documented AKS path and gives the scaler
    # chart everything it needs unchanged. See gpu_operator.tf.
    gpu_driver = "None"

    # Lets vm_size / os_disk overrides roll a fresh pool instead of failing.
    temporary_name_for_rotation = "tmpgpu"

    node_labels = {
      "keda-gpu-scaler.io/pool" = "gpu"
    }

    tags = local.tags

    # NOTE: intentionally the (untainted) default/system pool. KEDA, the GPU
    # operator controllers and CoreDNS schedule on the GPU node alongside the
    # scaler. The scaler chart tolerates `nvidia.com/gpu` regardless, so tainting
    # later is safe for the scaler but would strand system pods unless you add a
    # separate CPU pool.
  }

  identity {
    type = "SystemAssigned"
  }

  tags = local.tags
}
