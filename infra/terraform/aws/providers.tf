provider "aws" {
  region = var.region

  # Tag every resource so a forgotten cluster is easy to find (and bulk-delete).
  default_tags {
    tags = local.tags
  }
}

# The Kubernetes and Helm providers authenticate to the freshly created EKS
# cluster using the AWS CLI's `eks get-token` exec credential plugin. Tokens are
# short-lived and refreshed on every Terraform operation, so nothing needs to be
# written to ~/.kube/config for `apply` to work.
#
# Requirements on the machine running Terraform:
#   - awscli v2 on PATH (for `aws eks get-token`)
#   - valid AWS credentials for the same account/region as the cluster
provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

  exec {
    api_version = "client.authentication.k8s.io/v1"
    command     = "aws"
    args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name, "--region", var.region]
  }
}

# Helm provider v3 takes its Kubernetes connection settings as an attribute
# object (`kubernetes = { ... }`), and `exec` is likewise an attribute object —
# this differs from the Kubernetes provider above, which still uses an `exec {}`
# block. See the v2 -> v3 upgrade guide.
provider "helm" {
  kubernetes = {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

    exec = {
      api_version = "client.authentication.k8s.io/v1"
      command     = "aws"
      args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name, "--region", var.region]
    }
  }
}
