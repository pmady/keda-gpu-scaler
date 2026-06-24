# HPSF Emerging Stage Proposal — keda-gpu-scaler

This document is a draft of the HPSF TAC project proposal issue.
Submit via: https://github.com/hpsfoundation/tac/issues/new?template=new-project-proposal.md

---

### 1. Name of Project

keda-gpu-scaler

### 2. Project Description

keda-gpu-scaler collects NVIDIA GPU metrics (utilization, memory, temperature, power, PCIe/NVLink bandwidth) and has two main uses:

1. **KEDA External Scaler** — a gRPC server that lets KEDA scale Kubernetes workloads based on live GPU metrics. No Prometheus or dcgm-exporter needed.

2. **`gpu-metrics` CLI** — same NVML metrics, but standalone. Runs on bare metal, inside SLURM jobs, or under Flux. Auto-detects the scheduler and scopes collection to the GPUs assigned to the current job.

Both use NVIDIA's NVML C library via `go-nvml`. The CLI produces identical output fields regardless of whether it's running on K8s, SLURM, or Flux — so you can directly compare GPU performance across environments.

Metrics collected per GPU: utilization, memory usage, temperature, power draw/limit, PCIe tx/rx, NVLink tx/rx. Output formats: table, JSON, CSV. Multi-arch (amd64, arm64).

### 3. Statement on Alignment with High Performance Software Foundation's Mission

Right now if you want GPU metrics on a SLURM cluster you use one set of tools, and on Kubernetes you use a completely different stack. There's no common tool that works across both. That's the gap this project fills.

It already integrates with Flux (an HPSF project) and SLURM — auto-detecting the scheduler, scoping to assigned GPUs, and including job context in the output. Having a GPU observability tool that natively speaks HPC schedulers seems like a useful addition to the HPSF ecosystem.

For HPC researchers it's also just simpler: `srun gpu-metrics` gives you per-GPU telemetry inside your training job without setting up Prometheus or dcgm-exporter.

On the Kubernetes side, the KEDA scaler provides GPU-aware autoscaling — scale inference pods up/down based on actual GPU load. Sites running both K8s and SLURM/Flux can use one tool across both.

### 4. Project Website (please provide a link)

[https://github.com/pmady/keda-gpu-scaler](https://github.com/pmady/keda-gpu-scaler)

Documentation: [https://pmady.github.io/keda-gpu-scaler](https://pmady.github.io/keda-gpu-scaler)

### 5. Open Source License (please provide a link)

SPDX Identifier: `Apache-2.0`
[LICENSE](https://github.com/pmady/keda-gpu-scaler/blob/main/LICENSE)

### 6. Code of Conduct (please provide a link)

[Code of Conduct](https://github.com/pmady/keda-gpu-scaler/blob/main/CODE_OF_CONDUCT.md)

### 7. Governance Practices (please provide a link)

[GOVERNANCE.md](https://github.com/pmady/keda-gpu-scaler/blob/main/GOVERNANCE.md)

Single-maintainer model for now:
- Pavan Madduri (@pmady) — creator, all merge authority
- Decisions happen in GitHub Issues and PR review
- Contributors open PRs, maintainer reviews and merges

### 8. Two Sponsors from the High Performance Software Foundation's Technical Advisory Committee

1. Axel Huebl ([@ax3l](https://github.com/ax3l)) — WarpX, GPU-accelerated simulations
2. _[Pending confirmation — Dave Godlove or Vanessa Sochat]_

### 9. What is the project's solution for source control?

GitHub: [https://github.com/pmady/keda-gpu-scaler](https://github.com/pmady/keda-gpu-scaler)

### 10. What is the project's solution for issue tracking?

GitHub Issues: [https://github.com/pmady/keda-gpu-scaler/issues](https://github.com/pmady/keda-gpu-scaler/issues)

### 11. Please list all external dependencies and their license

| Dependency | License | Purpose |
|---|---|---|
| `github.com/NVIDIA/go-nvml` | Apache-2.0 | NVML Go bindings for GPU metrics |
| `go.uber.org/zap` | MIT | Structured logging |
| `github.com/stretchr/testify` | MIT | Test assertions |
| `google.golang.org/grpc` | Apache-2.0 | gRPC server for KEDA external scaler |
| `google.golang.org/protobuf` | BSD-3-Clause | Protocol buffer support |
| `github.com/prometheus/client_golang` | Apache-2.0 | Prometheus metrics endpoint |

### 12. Please describe your release methodology and mechanics

Releases are automated via GitHub Actions:
- Maintainers push a semver tag (`vX.Y.Z`) to `main`
- A `validate-tag` job rejects non-semver tags
- Multi-arch Docker images (`linux/amd64`, `linux/arm64`) are built and pushed to GHCR
- Release binaries for both architectures are compiled with CGO (for NVML linking)
- A GitHub Release is created with auto-generated changelog, attached binaries, and SHA256 checksums

### 13. Please describe Software Quality efforts (CI, security, auditing)

- **CI**: GitHub Actions runs build, unit tests, e2e tests, and `golangci-lint` on every push and PR, for both amd64 and arm64
- **Security**: OpenSSF Scorecard runs on every push. GitHub CodeQL performs static analysis. Dependencies are monitored by Dependabot
- **Testing**: Table-driven Go tests with `testify/assert`. Mock GPU collector enables full test coverage without NVIDIA hardware. Race detector (`-race`) enabled in CI
- **Code Review**: All changes go through PR review. DCO sign-off required on commits

### 14. Please list the project's leadership team

- **Pavan Madduri** ([@pmady](https://github.com/pmady)) — Creator, maintainer. Day job: Senior Platform Engineer at W.W. Grainger. Also contribute to KEDA, Volcano, Dragonfly, and OpenTelemetry.

### 15. Please list the project members with access to commit to the mainline of the project

- Pavan Madduri ([@pmady](https://github.com/pmady)) — maintainer, write access

Active contributors:
- Venkata Edara ([@venkata22a](https://github.com/venkata22a)) — CI/CD, Flux integration

### 16. Please describe the project's decision-making process

Features get proposed as GitHub Issues, discussed there, then implemented via PRs. PRs need maintainer approval to merge. Pretty standard single-maintainer workflow for now — will formalize roles if/when we get more regular contributors.

### 17. What is the maturity level of your project?

Emerging. Current release is v0.4.0, working on v0.5.0 (cross-environment metrics parity). Small but active contributor base. We think HPSF is the right home for the HPC side of this project.

### 18. Please list the project's official communication channels

- GitHub Issues and Discussions: [https://github.com/pmady/keda-gpu-scaler/issues](https://github.com/pmady/keda-gpu-scaler/issues)
- GitHub Pull Requests for technical discussion

### 19. Please list the project's social media accounts

- N/A (project communications happen on GitHub)

### 20. Please describe any existing financial sponsorships

None. Volunteer-maintained, no corporate backing.

### 21. Please describe the project's infrastructure needs or requests

Currently running fine on GitHub Actions free tier, GHCR for images, GitHub Pages for docs. The one thing we'd eventually want is GPU-enabled CI runners — right now all GPU tests use a mock collector. Being able to run integration tests against real NVML on a SLURM or Flux cluster would be great.

---

## TAC Presentation Notes

_Items to prepare for the TAC meeting presentation:_

### Live Demo Plan

1. **Kubernetes demo** (30s): Show `gpu-metrics` running inside a K8s pod, collecting A100 metrics, KEDA scaling a deployment based on GPU utilization

2. **SLURM demo** (30s): Show `srun --gres=gpu:2 gpu-metrics --format json` — automatic detection of SLURM environment, metrics scoped to assigned GPUs, job context in output

3. **Flux demo** (30s): Show `flux run -g 1 gpu-metrics --format json` — same output schema as SLURM, different scheduler context

4. **Cross-environment comparison** (30s): Side-by-side JSON output from K8s, SLURM, and Flux showing identical metric fields — the core value proposition of #54

### Key Talking Points

- "One binary, same metrics, any environment" — the pitch
- Complements Flux (HPSF project) — native integration already shipped
- No Prometheus, no dcgm-exporter, no heavy dependencies — just NVML
- KEDA ecosystem (CNCF) + HPC schedulers (HPSF) = bridging two worlds
- Apache 2.0, actively maintained, growing contributor base
