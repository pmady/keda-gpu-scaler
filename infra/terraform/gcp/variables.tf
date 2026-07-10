###############################################################################
# Cluster / GCP
###############################################################################

variable "project_id" {
  description = "GCP project ID to deploy the test cluster into."
  type        = string
}

variable "region" {
  description = "GCP region for the network/subnet. Pick one with GPU quota."
  type        = string
}

variable "zone" {
  description = "GCP zone the GKE cluster and GPU node pool run in (single zone, not regional, for predictable GPU cost/quota)."
  type        = string
}

variable "cluster_name" {
  description = "Name of the GKE cluster and the prefix used for the VPC and related resources."
  type        = string
}

variable "kubernetes_version" {
  description = "GKE control plane minimum version (<major>.<minor>); must be currently offered in your zone/release channel."
  type        = string
}

variable "subnet_cidr" {
  description = "IPv4 CIDR block for the GKE subnet's primary (node) IP range."
  type        = string
  default     = "10.0.0.0/20"
}

variable "pods_cidr" {
  description = "IPv4 CIDR block for the subnet's secondary range used for Pod IPs (VPC-native/alias IP)."
  type        = string
  default     = "10.4.0.0/14"
}

variable "services_cidr" {
  description = "IPv4 CIDR block for the subnet's secondary range used for Service IPs (VPC-native/alias IP)."
  type        = string
  default     = "10.8.0.0/20"
}

variable "labels" {
  description = "Extra labels merged into the default labels applied to every resource (e.g. an owner or expiry date)."
  type        = map(string)
}

###############################################################################
# Node pools
###############################################################################

variable "gpu_machine_type" {
  description = "GPU node machine type."
  type        = string
  default     = "n1-standard-4"
}

variable "gpu_type" {
  description = "GPU accelerator type attached to the n1 GPU nodes."
  type        = string
  default     = "nvidia-tesla-t4"
}

variable "gpu_per_node" {
  description = "Number of GPUs attached per GPU node."
  type        = number
  default     = 1
}

variable "gpu_node_count" {
  description = "Number of GPU nodes (fixed-size pool for predictable, low-cost integration testing)."
  type        = number
  default     = 1
}

variable "gpu_node_disk_size" {
  description = "Root disk size (GiB) per GPU node; generous by default since GPU images + driver/CUDA layers are large."
  type        = number
  default     = 100
}

###############################################################################
# Add-ons: NVIDIA GPU operator, KEDA, and the in-tree keda-gpu-scaler chart
###############################################################################

variable "gpu_operator_chart_version" {
  description = "NVIDIA GPU operator Helm chart version (repo https://helm.ngc.nvidia.com/nvidia)."
  type        = string
  default     = "v26.3.2"
}

variable "keda_chart_version" {
  description = "KEDA Helm chart version (repo https://kedacore.github.io/charts)."
  type        = string
  default     = "2.20.1"
}

variable "keda_namespace" {
  description = "Namespace KEDA and the keda-gpu-scaler are installed into."
  type        = string
  default     = "keda"
}

variable "scaler_release_name" {
  description = "Helm release name for the in-tree keda-gpu-scaler chart. Also determines the in-cluster service name / gRPC endpoint."
  type        = string
  default     = "keda-gpu-scaler"
}

variable "scaler_image_repository" {
  description = "Override the scaler image repository; empty string uses the chart default (ghcr.io/pmady/keda-gpu-scaler)."
  type        = string
}

variable "scaler_image_tag" {
  description = "Scaler container image tag to deploy. The chart appVersion has no published image, so pin a real tag (a vX.Y.Z release or `latest`)."
  type        = string
  default     = "v0.5.0"
}

variable "scaler_runtime_class_name" {
  description = "Override the scaler pod's runtimeClassName; null uses the chart default ('nvidia')."
  type        = string
  default     = null
}

variable "helm_timeout" {
  description = "Per-release Helm timeout in seconds. Bounds the KEDA install wait and the graceful `helm uninstall` on destroy (the GPU operator teardown is slow). Keep it generous so a slow uninstall doesn't fail and strand the billing GPU node."
  type        = number
}
