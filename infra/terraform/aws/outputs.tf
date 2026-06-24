output "cluster_name" {
  description = "EKS cluster name."
  value       = module.eks.cluster_name
}

output "region" {
  description = "AWS region the cluster runs in."
  value       = var.region
}

output "cluster_endpoint" {
  description = "EKS Kubernetes API server endpoint."
  value       = module.eks.cluster_endpoint
}

output "configure_kubectl" {
  description = "Command to write a kubeconfig entry for the new cluster."
  value       = "aws eks update-kubeconfig --region ${var.region} --name ${module.eks.cluster_name}"
}

output "scaler_namespace" {
  description = "Namespace KEDA and keda-gpu-scaler are installed in."
  value       = var.keda_namespace
}

output "scaler_grpc_endpoint" {
  description = "In-cluster gRPC address a KEDA ScaledObject external trigger should target (the `scalerAddress` metadata field)."
  value       = "${var.scaler_release_name}.${var.keda_namespace}.svc.cluster.local:6000"
}
