# AAIF Project Proposal — keda-gpu-scaler

Draft proposal for the Agentic AI Foundation. Submit via: https://github.com/aaif/project-proposals/issues/new

**Target submission:** Late June 2026 (after HPSF submission)

---

## Project Name

keda-gpu-scaler

## Project Description

A KEDA external scaler that reads GPU metrics via NVML and autoscales Kubernetes workloads based on actual GPU load. Runs as a DaemonSet on GPU nodes, serves metrics over gRPC to KEDA.

The short version: Kubernetes HPA can't see GPU utilization. A vLLM pod can be serving 200 requests at 8% CPU while the GPU is pegged. The usual fix is dcgm-exporter → Prometheus → PromQL → KEDA, which works but adds 15-30s of latency and 5 moving parts. This skips all of that: it reads NVML directly when KEDA asks, so metric age is just KEDA's polling interval.

Includes scaling profiles for vLLM, Triton, training, and batch inference so you don't have to guess the right thresholds.

## How does this project align with the AAIF mission?

This isn't an agent framework — it's the infrastructure underneath. When an agent makes a bunch of LLM calls, something has to scale the inference backend. Fixed replica counts either waste GPUs or create queuing delays.

This project handles GPU-based autoscaling so inference pods scale up when GPUs are loaded and scale back down (including to zero) when they're idle. It's already running in production on A100 clusters for vLLM serving.

## Project Website

https://github.com/pmady/keda-gpu-scaler

Documentation: https://pmady.github.io/keda-gpu-scaler

## Open Source License

Apache-2.0: https://github.com/pmady/keda-gpu-scaler/blob/main/LICENSE

## Code of Conduct

https://github.com/pmady/keda-gpu-scaler/blob/main/CODE_OF_CONDUCT.md

## Governance

https://github.com/pmady/keda-gpu-scaler/blob/main/GOVERNANCE.md

Single-maintainer model. Decisions happen in GitHub Issues and PR review.

## Source Control

GitHub: https://github.com/pmady/keda-gpu-scaler

## Issue Tracking

GitHub Issues: https://github.com/pmady/keda-gpu-scaler/issues

## External Dependencies

| Dependency | License | Purpose |
|---|---|---|
| github.com/NVIDIA/go-nvml | Apache-2.0 | GPU metrics via NVML |
| google.golang.org/grpc | Apache-2.0 | KEDA external scaler interface |
| go.uber.org/zap | MIT | Logging |
| github.com/prometheus/client_golang | Apache-2.0 | Optional metrics endpoint |

## Release Methodology

Automated via GitHub Actions:
- Semver tags trigger releases
- Multi-arch Docker images (amd64, arm64) pushed to GHCR
- Binaries attached to GitHub Releases with checksums

## Software Quality

- CI: Build, test, lint on every PR (amd64 + arm64)
- Security: OpenSSF Scorecard, CodeQL, Dependabot
- Testing: Table-driven Go tests, mock GPU collector, race detector
- Code review: All changes via PR, DCO sign-off required

## Project Leadership

- **Pavan Madduri** ([@pmady](https://github.com/pmady)) — Creator, maintainer

## Commit Access

- Pavan Madduri (@pmady) — maintainer

## Decision-Making Process

Features proposed as GitHub Issues, discussed, implemented via PRs. Maintainer approval required to merge.

## Project Maturity

v0.4.0 released, running in production on A100 clusters.

## Communication Channels

- GitHub Issues and Discussions
- GitHub Pull Requests

## Social Media

N/A — project communications on GitHub

## Financial Sponsorships

None. Volunteer-maintained.

## Infrastructure Needs

Currently using GitHub Actions free tier, GHCR, GitHub Pages. No immediate needs. GPU-enabled CI runners would be nice eventually for integration testing.

---

## Notes for TC Presentation

Not an agent framework — the infra that agent deployments need. Demo: KEDA scaling a vLLM deployment on GPU memory pressure, 30 seconds. Key point: no Prometheus pipeline, sub-5s latency, profiles for LLM serving out of the box.
