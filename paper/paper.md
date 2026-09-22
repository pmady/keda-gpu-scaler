---
title: 'keda-gpu-scaler: GPU-aware autoscaling for Kubernetes from real hardware metrics'
tags:
  - Kubernetes
  - GPU
  - autoscaling
  - KEDA
  - NVML
  - Go
  - machine learning inference
authors:
  - name: Pavan Madduri
    orcid: 0009-0007-3795-7593
    affiliation: 1
affiliations:
  - name: Independent Researcher, USA
    index: 1
date: 20 September 2026
bibliography: paper.bib
---

# Summary

`keda-gpu-scaler` is a Kubernetes External Scaler for KEDA [@keda] that drives
horizontal autoscaling of GPU workloads from GPU hardware metrics. It runs as a
DaemonSet on GPU nodes, reads per-device state directly from the NVIDIA
Management Library (NVML) [@nvml], and serves those metrics to KEDA over the
external scaler gRPC contract. Workloads can then scale on GPU utilization,
memory use, or engine-level signals, including scaling to and from zero
replicas. Beyond raw device metrics, the scaler can read serving-engine signals
such as vLLM [@kwon2023vllm] queue depth and NVIDIA Triton queue statistics, so
scaling can track request pressure rather than only device saturation. It
supports Multi-Instance GPU (MIG) partitions and several aggregation strategies
(max, min, average, sum, and percentiles) for nodes with multiple GPUs.

# Statement of need

The Kubernetes Horizontal Pod Autoscaler reacts to CPU and memory, neither of
which reflects GPU saturation. A GPU inference pod can sit at single-digit CPU
utilization while its GPU is fully saturated, so CPU- and memory-based
autoscaling either over-provisions or fails to respond. The common workaround
chains `dcgm-exporter` into Prometheus, evaluates PromQL, and feeds the result
back through KEDA. That pipeline adds several moving parts and tens of seconds of
metric latency, which is significant when GPU capacity is expensive and demand is
bursty.

`keda-gpu-scaler` removes that pipeline. Because it reads NVML on the node and
answers KEDA directly, scaling decisions are based on live hardware state without
an intermediate metrics store or query language. This matters for large language
model serving and other GPU inference workloads, where operators need to scale
on GPU utilization or on serving-engine backpressure, and where scale-to-zero
meaningfully reduces cost. The project targets platform and machine learning
infrastructure engineers who run GPU workloads on Kubernetes and want autoscaling
that reflects what the accelerator is actually doing.

# State of the field

GPU metric-driven autoscaling in Kubernetes is typically achieved through a
multi-stage pipeline. NVIDIA's `dcgm-exporter` [@dcgm] scrapes GPU counters
into Prometheus, PromQL queries are evaluated by a Prometheus adapter or by
KEDA's Prometheus scaler, and the result drives the Horizontal Pod Autoscaler.
The NVIDIA GPU Operator [@gpuoperator] automates driver and device-plugin
lifecycle but delegates scaling to this same Prometheus path. GKE's
`custom-metrics-stackdriver-adapter` and similar managed-cloud solutions exist
but are vendor-locked and unavailable on bare-metal or multi-cloud clusters.

`keda-gpu-scaler` was built as a new project rather than contributed to these
existing tools for architectural reasons. KEDA's external-scaler gRPC contract
is the only extension point that allows a node-local daemon to push live
hardware state directly into the scaling loop without an intermediate time-series
database. Embedding GPU logic into KEDA core would couple KEDA's release cycle
to NVML and vendor-specific dependencies; the KEDA maintainers have explicitly
recommended external scalers for hardware-specific metrics. Contributing to
`dcgm-exporter` would not eliminate the Prometheus dependency, which is the
primary source of latency and operational complexity that this project removes.

# Software design

`keda-gpu-scaler` runs as a Kubernetes DaemonSet: one pod per GPU node, with
direct access to the NVIDIA Management Library through `go-nvml` CGo bindings.
This node-local architecture was chosen over a centralized controller because
GPU state is per-host and NVML requires local device access.

The core abstraction is a `Collector` interface that decouples metric
acquisition from the gRPC serving layer. The default implementation calls NVML;
a vendor-agnostic factory (contributed by an external developer) selects the
appropriate collector at startup, enabling future support for AMD ROCm or Intel
Level Zero without changing the server. A mock collector enables full test
coverage without GPU hardware.

The gRPC server implements KEDA's four-method `ExternalScaler` contract.
Scaling decisions are configured entirely through `ScaledObject` trigger
metadata — no custom resources or operator are required. Aggregation strategy
(max, average, percentile) is a runtime parameter, allowing operators to tune
fleet-wide scaling behavior without code changes.

Serving-engine integration (vLLM, Triton) is implemented as composable metric
sources that layer on top of device metrics, so scaling can combine hardware
saturation with request-queue pressure in a single `ScaledObject`.

# Research impact statement

`keda-gpu-scaler` is deployed in production at a Fortune 500 industrial
distributor, where it autoscales vLLM inference workloads on a four-node A100
cluster, replacing a dcgm-exporter/Prometheus pipeline that added approximately
30 seconds of metric latency. The project's design was presented in a CNCF TAG
Runtime whitepaper on GPU-aware autoscaling [@cncfwhitepaper] and discussed in
an IEEE Communications Society Technology Blog article on scaling agentic AI in
autonomous telecom networks [@comsocblog].

The repository has 130 stars, 36 forks, and contributions from multiple
external developers across organizations, including a vendor-agnostic GPU
collector factory contributed by an independent developer (PR #222). The project
has eight tagged releases spanning twelve months of iterative development, with
305 non-merge commits, CI on every pull request, Dependabot for dependency
management, and OpenSSF Scorecard and CodeQL security analysis enabled. A
Terraform-based infrastructure-as-code stack for reproducible GPU cluster
provisioning is included and tested via Terratest end-to-end tests, contributed
by another external collaborator.

# Functionality

The scaler exposes the four methods of the KEDA external scaler interface and is
configured entirely through standard `ScaledObject` trigger metadata. Key
capabilities include:

- Per-device metrics from NVML: utilization, memory used and total, temperature,
  and power draw.
- Serving-engine metrics: vLLM pending queue depth and KV-cache usage, and
  Triton queue statistics, for request-aware scaling.
- Multi-GPU aggregation strategies (`max`, `min`, `avg`, `sum`, `p95`, `p99`).
- MIG per-instance metrics for partitioned GPUs.
- Scale-to-zero and configurable cooldown to prevent scale-down flapping.
- An optional Prometheus endpoint for fleet monitoring, independent of the
  scaling path.

The design and rationale, including why GPU support is implemented as a
standalone external scaler rather than embedded in KEDA core, are documented in
the repository.

# AI usage disclosure

GitHub Copilot was used for code-completion assistance during development, and
Claude (Anthropic) was used to assist with drafting documentation and this
paper. All code, design decisions, architectural choices, and paper content were
reviewed, validated, and edited by the human author. The core design — using a
DaemonSet with direct NVML access behind KEDA's external-scaler gRPC contract —
was conceived and implemented by the author based on production experience with
GPU inference workloads.

# Acknowledgements

The project builds on the KEDA project [@keda], the NVIDIA Management Library
[@nvml], and the broader Kubernetes ecosystem [@burns2016borg].

# References
