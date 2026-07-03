# AGENTS.md — `infra/` guidance

Guidance for humans and AI agents working on the Infrastructure-as-Code in this
directory. Keep changes consistent with the conventions below.

## Purpose

`infra/` provisions **throwaway, GPU-ready Kubernetes clusters** for
integration-testing `keda-gpu-scaler` against **real NVIDIA hardware**. The Go
test suite only exercises a mock GPU collector, so these clusters are how the
scaler is validated end-to-end on an actual GPU. They are test clusters, not
production infrastructure.

## Follow the project's contribution rules

This file **supplements, it does not replace** the repository's contribution
policy. Read and follow both:

- **[CONTRIBUTING.md](../CONTRIBUTING.md)** — workflow, build/test, pre-commit
  hooks, code style, releases, DCO.
- **[AI_GUIDELINES.md](../AI_GUIDELINES.md)** — policy for AI-assisted work.

The rules that apply to any change here:

- **Human-verify everything.** Every AI-assisted change must be reviewed,
  understood, and tested before submitting — you own what you commit.
- **Run the checks before pushing.** Install the pre-commit hooks
  (`pre-commit install` — catches trailing whitespace, missing final newlines,
  formatting). For Go changes run `make fmt` / `make test` / `make lint`; for
  Terraform run `terraform fmt` / `terraform validate`.
- **Match the house style.** Terse, single-line comments; no verbose godoc or
  over-polished prose; table-driven tests.
- **One logical change per PR.** Don't let tooling expand scope into adjacent
  code (one cloud / one concern per PR).
- **Disclose significant AI usage** in the PR description.
- **Sign off every commit** (`git commit -s`, or `git rebase --signoff`) — the
  CI DCO check enforces it.

## Layout

```
infra/
  terraform/
    aws/        # Amazon EKS (implemented)
    azure/      # Azure AKS  (implemented)
    # gcp/      # Google GKE (planned)
    README.md   # index + per-cloud details
```

Each cloud is a **self-contained, independently `apply`-able** Terraform stack
(its own providers, modules, variables, and state). They do not share a root
module, so a new cloud is a sibling directory following the same conventions —
no rework of existing stacks.

## What every stack must do

One `terraform apply` produces a cluster that is immediately ready for tests,
with no manual post-apply steps:

1. a cluster + a small GPU node pool;
2. the GPU made usable in Kubernetes — driver, device plugin, the `nvidia`
   RuntimeClass, and the `nvidia.com/gpu.present` node label;
3. KEDA; then
4. `keda-gpu-scaler`, installed from the in-tree chart at
   `deploy/helm/keda-gpu-scaler`.

Provide consistent outputs (`configure_kubectl`, `scaler_grpc_endpoint`) and tag
all resources (`Project=keda-gpu-scaler`, `ManagedBy=terraform`) so a forgotten
cluster is easy to find.

## Scaler chart requirements

Always read `deploy/helm/keda-gpu-scaler/values.yaml` before wiring a cluster.
The scaler is a privileged DaemonSet that links `libnvidia-ml.so` at runtime, so
each GPU node must provide:

- node label `nvidia.com/gpu.present=true`
- `runtimeClassName: nvidia`
- a working NVIDIA driver + `libnvidia-ml.so`
- privileged execution

Satisfy these with the **NVIDIA GPU operator** (device plugin + GPU-feature-
discovery label + `nvidia` RuntimeClass) or by overriding the chart
(`runtimeClassName=""`, `nvmlHostMounts.enabled=true`, relaxed `nodeSelector`).
State which approach a stack uses and why.

## Conventions

- **Pin versions.** Confirm the latest Terraform release, provider versions,
  module versions, and resource schemas before writing — don't rely on memory.
  Add `.terraform-version` and floor `required_version` at the current minor.
- **Current Kubernetes version.** Default to a version still in standard support;
  never default to a near-EOL minor.
- **Fixed, predictable pool.** A single on-demand GPU node, no cluster
  autoscaler, no spot — predictable and cheap for tests.
- **Install add-ons explicitly.** Ensure the CNI and GPU device plugin are
  installed and the node reaches `Ready` and advertises `nvidia.com/gpu`.
- **Order KEDA before the scaler.** The chart renders a `ScaledObject`, so KEDA's
  CRDs must exist first (`depends_on`).
- **Set a real image tag.** The chart's `appVersion` may not have a matching
  published image; set `image.tag` to a published tag of
  `ghcr.io/pmady/keda-gpu-scaler` (e.g. `latest` or a `vX.Y.Z` release).
- **Line endings.** `.gitattributes` forces LF on `*.sh`/`*.yaml`/`*.tf` so
  Windows checkouts don't break script shebangs.

## Scope: pods, not GPU nodes

`keda-gpu-scaler` scales workload **pods** based on GPU load. Scaling GPU
**nodes** is intentionally out of scope (use Karpenter / Cluster Autoscaler or
the cloud equivalent). Test clusters therefore use a fixed GPU node count and no
node autoscaler; the demos in `infra/terraform/<cloud>/demo/` show pod scale
up/down only.

## GPU service quota (all clouds)

Cloud GPU quota is typically **0 on fresh accounts**, and is per-region and
per-GPU-family. Identify the exact quota and request an increase **before**
applying, or apply fails at node creation. Document the quota name, running cost,
and `terraform destroy` loudly in each stack's README.

- AWS: "Running On-Demand G and VT instances" (`L-DB2E81BA`), measured in vCPUs.
- Azure: per-VM-family vCPU quota (NC / ND / NV families).
- GCP: a global GPU quota plus per-region, per-type GPU quotas.

## Cost & teardown

GPU clusters bill by the hour. Always `terraform destroy` after testing and
remind users to do so. Resource tags make any leftovers findable.

## Validation (before claiming a stack works)

Apply on real hardware and confirm:

- the scaler pod is `1/1 Running`; its logs show NVML initialized and the GPU
  being read over gRPC;
- the `ScaledObject` is `READY` / `ACTIVE`;
- under GPU load (`infra/terraform/<cloud>/demo/gpu-load.yaml`) the demo
  Deployment scales up and then back down when idle.

## Adding a new cloud

- Create `infra/terraform/<cloud>/` mirroring `infra/terraform/aws/` — structure,
  variables, outputs, and a "loud" README covering quota, cost, and teardown.
- Use a well-maintained community cluster module; pin it.
- Default to a cheap, current-generation GPU SKU, easily overridable.
- Work on a dedicated branch (e.g. `feat/<cloud>-gpu-test-infra`), follow the
  contribution rules above (pre-commit + checks, terse style, DCO sign-off, AI
  disclosure), and open one PR per cloud.
