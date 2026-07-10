locals {
  # GCP label values must be lowercase alphanumeric (dashes/underscores ok).
  labels = merge(
    {
      project    = "keda-gpu-scaler"
      component  = "gpu-integration-test"
      managed-by = "terraform"
      stack      = "infra-terraform-gcp"
    },
    var.labels,
  )
}

###############################################################################
# Networking
###############################################################################

resource "google_compute_network" "vpc" {
  name                    = var.cluster_name
  auto_create_subnetworks = false
}

resource "google_compute_subnetwork" "subnet" {
  name          = "${var.cluster_name}-subnet"
  region        = var.region
  network       = google_compute_network.vpc.id
  ip_cidr_range = var.subnet_cidr

  # Secondary ranges GKE uses for VPC-native (alias IP) pod and Service IPs.
  secondary_ip_range {
    range_name    = "pods"
    ip_cidr_range = var.pods_cidr
  }
  secondary_ip_range {
    range_name    = "services"
    ip_cidr_range = var.services_cidr
  }
}

###############################################################################
# GKE control plane + single untainted GPU node pool
###############################################################################

resource "google_container_cluster" "primary" {
  name = var.cluster_name

  # Zonal, not regional: keeps GPU cost/quota predictable for a throwaway cluster.
  location = var.zone

  # Node pools managed as separate resources below, not the built-in default pool.
  remove_default_node_pool = true
  initial_node_count       = 1

  network    = google_compute_network.vpc.id
  subnetwork = google_compute_subnetwork.subnet.id

  networking_mode = "VPC_NATIVE"
  ip_allocation_policy {
    cluster_secondary_range_name  = "pods"
    services_secondary_range_name = "services"
  }

  min_master_version = var.kubernetes_version

  # So `terraform destroy` works on this throwaway cluster.
  deletion_protection = false

  resource_labels = local.labels
}

resource "google_container_node_pool" "gpu" {
  name       = "gpu"
  cluster    = google_container_cluster.primary.id
  location   = var.zone
  node_count = var.gpu_node_count

  node_config {
    machine_type = var.gpu_machine_type

    # Ubuntu, not COS — the GPU operator's driver container needs it.
    image_type   = "UBUNTU_CONTAINERD"
    disk_size_gb = var.gpu_node_disk_size
    oauth_scopes = ["https://www.googleapis.com/auth/cloud-platform"]

    labels = {
      "keda-gpu-scaler.io/pool" = "gpu"

      # Disable GKE's own GPU device plugin so it doesn't conflict with the operator's.
      "gke-no-default-nvidia-gpu-device-plugin" = "true"
    }

    guest_accelerator {
      type  = var.gpu_type
      count = var.gpu_per_node

      # INSTALLATION_DISABLED: GKE installs no GPU software; the NVIDIA GPU operator
      # owns the driver + toolkit (see gpu_operator.tf). The node then stays untainted.
      gpu_driver_installation_config {
        gpu_driver_version = "INSTALLATION_DISABLED"
      }
    }
  }

  management {
    auto_repair  = true
    auto_upgrade = false
  }
}
