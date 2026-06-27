###############################################################################
# Cluster / AWS
###############################################################################

variable "region" {
  description = "AWS region to deploy the test cluster into. Pick one with GPU capacity (and where you hold the GPU service quota)."
  type        = string
}

variable "cluster_name" {
  description = "Name of the EKS cluster and the prefix used for the VPC and related resources."
  type        = string
}

variable "kubernetes_version" {
  description = "EKS Kubernetes control plane version (<major>.<minor>). Latest on EKS is 1.36; pick a version still in standard support."
  type        = string
}

variable "vpc_cidr" {
  description = "IPv4 CIDR block."
  type        = string
  default     = "10.0.0.0/16"
}

variable "tags" {
  description = "Extra tags merged into the default tags applied to every resource (e.g. an owner or expiry date)."
  type        = map(string)
}

###############################################################################
# GPU node pool
###############################################################################

variable "gpu_instance_type" {
  description = "GPU instance type"
  type        = string
}

variable "gpu_ami_type" {
  description = "EKS-optimized Amazon Linux 2023 with NVIDIA hsot driver"
  type        = string
  default     = "AL2023_x86_64_NVIDIA"

  validation {
    condition     = can(regex("NVIDIA|GPU", var.gpu_ami_type))
    error_message = "gpu_ami_type must be an NVIDIA/GPU accelerated EKS AMI type (e.g. AL2023_x86_64_NVIDIA, BOTTLEROCKET_x86_64_NVIDIA, AL2_x86_64_GPU)."
  }
}

variable "gpu_node_count" {
  description = "Number of GPU nodes. Fixed-size on-demand pool (min = max = desired). Kept at 1 for predictable, low-cost integration testing."
  type        = number
}

variable "gpu_node_disk_size" {
  description = "Root EBS volume size (GiB) per GPU node. GPU container images plus the driver/CUDA layers are large, so this is generous by default."
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
  description = "Override the scaler container image repository. Empty string uses the chart default (ghcr.io/pmady/keda-gpu-scaler)."
  type        = string
}

variable "scaler_image_tag" {
  description = "Scaler container image tag to deploy"
  type        = string
  default     = "v0.5.0"
}

variable "scaler_runtime_class_name" {
  description = "Override the scaler pod's runtimeClassName. null uses the chart default ('nvidia')"
  type        = string
  default     = null
}

variable "helm_timeout" {
  description = "Per-release Helm wait timeout in seconds. Generous because GPU driver/device-plugin rollout and node labelling can take several minutes."
  type        = number
}
