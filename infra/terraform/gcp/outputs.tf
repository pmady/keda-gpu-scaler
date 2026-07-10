output "cluster_name" {
  description = "GKE cluster name."
  value       = google_container_cluster.primary.name
}

output "location" {
  description = "GCP zone the cluster runs in."
  value       = var.zone
}

output "cluster_endpoint" {
  description = "GKE Kubernetes API server endpoint."
  value       = "https://${google_container_cluster.primary.endpoint}"
}

output "configure_kubectl" {
  description = "Command to write a kubeconfig entry for the new cluster."
  value       = "gcloud container clusters get-credentials ${google_container_cluster.primary.name} --zone ${var.zone} --project ${var.project_id}"
}

output "scaler_namespace" {
  description = "Namespace KEDA and keda-gpu-scaler are installed in."
  value       = var.keda_namespace
}

output "scaler_grpc_endpoint" {
  description = "In-cluster gRPC address a KEDA ScaledObject external trigger should target (the `scalerAddress` metadata field)."
  value       = "${var.scaler_release_name}.${var.keda_namespace}.svc.cluster.local:6000"
}
