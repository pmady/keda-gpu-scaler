provider "azurerm" {
  # subscription_id is required by the azurerm v4 provider. Leave var.subscription_id
  # null to fall back to the ARM_SUBSCRIPTION_ID environment variable.
  subscription_id = var.subscription_id

  features {}
}

# The Kubernetes and Helm providers authenticate to the freshly created AKS
# cluster using the admin kubeconfig that azurerm exports from the cluster
# resource (host + client cert/key + CA). These are read straight from state, so
# nothing needs to be written to ~/.kube/config for `apply` to work.
#
# This relies on the cluster's local admin account (local_account_disabled =
# false, the default). The `az` CLI is NOT required by Terraform itself — only
# to fetch a kubeconfig afterwards (see the configure_kubectl output).
provider "kubernetes" {
  host                   = azurerm_kubernetes_cluster.this.kube_config[0].host
  client_certificate     = base64decode(azurerm_kubernetes_cluster.this.kube_config[0].client_certificate)
  client_key             = base64decode(azurerm_kubernetes_cluster.this.kube_config[0].client_key)
  cluster_ca_certificate = base64decode(azurerm_kubernetes_cluster.this.kube_config[0].cluster_ca_certificate)
}

# Helm provider v3 takes its Kubernetes connection settings as an attribute
# object (`kubernetes = { ... }`) rather than a nested block — see the v2 -> v3
# upgrade guide.
provider "helm" {
  kubernetes = {
    host                   = azurerm_kubernetes_cluster.this.kube_config[0].host
    client_certificate     = base64decode(azurerm_kubernetes_cluster.this.kube_config[0].client_certificate)
    client_key             = base64decode(azurerm_kubernetes_cluster.this.kube_config[0].client_key)
    cluster_ca_certificate = base64decode(azurerm_kubernetes_cluster.this.kube_config[0].cluster_ca_certificate)
  }
}
