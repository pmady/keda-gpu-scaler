# Infrastructure-as-Code for keda-gpu-scaler

Terraform for standing up **throwaway** GPU-ready Kubernetes clusters to
integration-test `keda-gpu-scaler` against real NVIDIA hardware. These are test
clusters, not production infrastructure.

## Layout

```
infra/terraform/
  aws/        # Amazon EKS (implemented)   -> see aws/README.md
  # gcp/      # Google GKE  (planned)
  # azure/    # Azure AKS   (planned)
  # kind/     # local kind + mock NVML (planned)
```

Each cloud lives in its own self-contained, independently `apply`-able directory
(its own providers, modules, variables, state). They deliberately do **not**
share a root module, so adding GCP/Azure/kind later is a matter of dropping in a
sibling directory that follows the same convention — no rework of the AWS stack.

The shared contract every directory aims to honour:

- one `terraform apply` produces a cluster that is immediately ready for
  integration tests (GPU drivers + device plugin, KEDA, and `keda-gpu-scaler`
  installed from the in-tree chart at `deploy/helm/keda-gpu-scaler`);
- the same `*_grpc_endpoint` / `configure_kubectl` style outputs;
- resources tagged/labelled so a forgotten cluster is easy to find and destroy.

## Status

| Target | Directory | Status |
|---|---|---|
| AWS EKS | [`aws/`](./aws) | ✅ Implemented |
| GCP GKE | `gcp/` | ⏳ Planned |
| Azure AKS | `azure/` | ⏳ Planned |
| Local kind + mock NVML | `kind/` | ⏳ Planned |

## Conventions

- **Terraform version** is pinned per directory via `.terraform-version`
  (currently `1.15.6`); `required_version` floors at the current minor.
- **Providers and community modules are version-pinned.** Versions were
  confirmed against the Terraform Registry at authoring time.
- **CI:** manual only for now — a human runs `terraform apply` locally. This is
  intentionally **not** wired into GitHub Actions.

Start with [`aws/README.md`](./aws/README.md) — it covers the GPU service-quota
prerequisite, cost, and teardown.
