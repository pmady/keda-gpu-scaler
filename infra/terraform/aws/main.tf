data "aws_availability_zones" "available" {
  state = "available"

  # Only zones that don't require explicit opt-in (skips Local/Wavelength zones).
  filter {
    name   = "opt-in-status"
    values = ["opt-in-not-required"]
  }
}

locals {
  # Spread the VPC across up to 3 AZs so the GPU node group can land wherever
  # the chosen instance type has capacity.
  azs = slice(data.aws_availability_zones.available.names, 0, 3)

  tags = merge(
    {
      Project   = "keda-gpu-scaler"
      Component = "gpu-integration-test"
      ManagedBy = "terraform"
      Stack     = "infra/terraform/aws"
    },
    var.tags,
  )
}

###############################################################################
# Networking
###############################################################################

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "6.6.1"

  name = var.cluster_name
  cidr = var.vpc_cidr

  azs             = local.azs
  private_subnets = [for i in range(length(local.azs)) : cidrsubnet(var.vpc_cidr, 4, i)]
  public_subnets  = [for i in range(length(local.azs)) : cidrsubnet(var.vpc_cidr, 8, i + 48)]

  # Nodes live in private subnets and reach the internet (image pulls, the
  # NVIDIA NGC helm repo, etc.) via a single shared NAT gateway. One NAT gateway
  # keeps the test cluster cheap; it is not HA, which is fine for a throwaway.
  enable_nat_gateway   = true
  single_nat_gateway   = true
  enable_dns_hostnames = true

  public_subnet_tags = {
    "kubernetes.io/role/elb" = "1"
  }
  private_subnet_tags = {
    "kubernetes.io/role/internal-elb" = "1"
  }
}

###############################################################################
# EKS control plane + GPU node group
###############################################################################

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "21.23.0"

  name               = var.cluster_name
  kubernetes_version = var.kubernetes_version

  # Public API endpoint so an operator can reach the cluster with a kubeconfig
  # from their laptop. This is a throwaway test cluster; restrict this for
  # anything that outlives a test run.
  endpoint_public_access = true

  # Add the identity running `terraform apply` as a cluster admin via an EKS
  # access entry, so the kubernetes/helm providers can install the add-ons in
  # the same apply.
  enable_cluster_creator_admin_permissions = true

  vpc_id     = module.vpc.vpc_id
  subnet_ids = module.vpc.private_subnets

  eks_managed_node_groups = {
    gpu = {
      ami_type       = var.gpu_ami_type
      instance_types = [var.gpu_instance_type]
      capacity_type  = "ON_DEMAND"

      # Fixed-size pool: predictable for tests, no autoscaling, no spot.
      min_size     = var.gpu_node_count
      max_size     = var.gpu_node_count
      desired_size = var.gpu_node_count

      disk_size = var.gpu_node_disk_size

      labels = {
        "keda-gpu-scaler.io/pool" = "gpu"
      }

      # NOTE: intentionally NOT tainted. This is a single-pool cluster, so KEDA,
      # the GPU operator controllers and CoreDNS must be able to schedule on the
      # GPU node too. The scaler chart tolerates `nvidia.com/gpu` regardless, so
      # adding a taint here later is safe for the scaler but would strand the
      # system/add-on pods unless you also add a separate CPU node group.
    }
  }
}
