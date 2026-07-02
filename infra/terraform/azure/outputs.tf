output "cluster_name" {
  description = "AKS cluster name."
  value       = azurerm_kubernetes_cluster.this.name
}

output "location" {
  description = "Azure region the cluster runs in."
  value       = var.location
}

output "resource_group_name" {
  description = "Resource group holding the cluster."
  value       = azurerm_resource_group.this.name
}

output "cluster_endpoint" {
  description = "AKS Kubernetes API server FQDN."
  value       = azurerm_kubernetes_cluster.this.fqdn
}

output "configure_kubectl" {
  description = "Command to write a kubeconfig entry for the new cluster."
  value       = "az aks get-credentials --resource-group ${azurerm_resource_group.this.name} --name ${azurerm_kubernetes_cluster.this.name} --overwrite-existing"
}

output "scaler_namespace" {
  description = "Namespace KEDA and keda-gpu-scaler are installed in."
  value       = var.keda_namespace
}

output "scaler_grpc_endpoint" {
  description = "In-cluster gRPC address a KEDA ScaledObject external trigger should target (the `scalerAddress` metadata field)."
  value       = "${var.scaler_release_name}.${var.keda_namespace}.svc.cluster.local:6000"
}
