# GPU-Aware Autoscaling in Cloud Native AI Infrastructure

**CNCF TAG Infrastructure — AI Technical Community Group Whitepaper**

**Author:** Pavan Madduri ([@pmady](https://github.com/pmady))
**Status:** Draft
**Created:** June 2026
**CNCF TOC Issue:** TBD
**Related Projects:** [KEDA](https://keda.sh), [Volcano](https://volcano.sh), [NVIDIA NVML](https://developer.nvidia.com/management-library-nvml)

---

## Abstract

Kubernetes has no idea what's happening on a GPU. A pod either gets one or it doesn't — there's no utilization awareness, no VRAM tracking, no thermal feedback. This is fine for CPU workloads but completely wrong for AI inference, where a pod can show 8% CPU while the GPU is pegged at 100%.

This paper covers how we built GPU-aware autoscaling on top of KEDA using NVIDIA NVML, why it has to run as a DaemonSet instead of living inside KEDA core, and what scaling profiles look like for real inference workloads like vLLM and Triton.

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [The GPU Scaling Blind Spot](#2-the-gpu-scaling-blind-spot)
3. [Why Native Integration Fails](#3-why-native-integration-fails)
4. [Architecture: Node-Level Telemetry via External gRPC Scalers](#4-architecture-node-level-telemetry-via-external-grpc-scalers)
5. [Scaling Profiles for AI Workloads](#5-scaling-profiles-for-ai-workloads)
6. [Multi-GPU Aggregation Strategies](#6-multi-gpu-aggregation-strategies)
7. [NUMA-Aware GPU Scheduling](#7-numa-aware-gpu-scheduling)
8. [Evaluation](#8-evaluation)
9. [Related Work](#9-related-work)
10. [Future Directions](#10-future-directions)
11. [Conclusion](#11-conclusion)
12. [References](#12-references)

---

## 1. Introduction

If you run GPU workloads on Kubernetes, you've probably noticed that HPA doesn't help. It watches CPU and memory. Your vLLM pod is serving 200 concurrent requests, the GPU is completely saturated, and HPA sees 8% CPU. It does nothing.

VPA doesn't help either. KEDA gets you closer with custom metrics, but the standard approach — DCGM exporter into Prometheus, then a PromQL query feeding KEDA's Prometheus scaler — adds 15-30 seconds of latency and a lot of moving parts.

keda-gpu-scaler takes a different approach: read GPU metrics directly from NVML on each node, serve them to KEDA over gRPC, skip the metrics pipeline entirely. It's a DaemonSet that reads NVML on each request from KEDA and implements KEDA's external scaler interface. It ships with scaling profiles for vLLM, Triton, training, and batch workloads, and handles multi-GPU nodes (4x A100, 8x H100) with configurable aggregation.

## 2. The GPU Scaling Blind Spot

### 2.1 The Current State

Kubernetes knows GPUs exist as extended resources (`nvidia.com/gpu`). You request a count, the scheduler assigns them, and that's it. There's no way to ask "how busy is this GPU?" or "how much VRAM is left?" from the autoscaler. An idle GPU and a saturated GPU look the same to HPA.

### 2.2 The Metrics Pipeline Problem

The conventional approach to GPU-based scaling chains multiple components:

```
DCGM Exporter → Prometheus → PromQL → Prometheus Adapter → HPA
```

or with KEDA:

```
DCGM Exporter → Prometheus → PromQL → KEDA Prometheus Scaler → HPA
```

This works, but it's ugly:

| Problem | Impact |
|---------|--------|
| **Metric latency** | 15-30 seconds from GPU state change to scaling decision due to scrape intervals and aggregation |
| **Operational complexity** | 5 components to deploy, configure, and maintain |
| **PromQL fragility** | Scaling logic encoded in PromQL strings — no type safety, hard to test |
| **Single point of failure** | Prometheus outage = no GPU scaling |
| **Resource overhead** | Prometheus retention, DCGM exporter per node, adapter deployment |

### 2.3 Why This Matters Now

GPU workloads don't scale like CPU workloads. LLM inference has hard latency SLAs — if you scale too late, requests get dropped. VRAM is usually the bottleneck, not compute — vLLM pre-allocates KV cache, so memory pressure is the real signal. And GPUs are expensive: an A100 costs $2-3/hour. Over-provisioning 4 GPUs costs more than over-provisioning 40 CPU cores.

There's also the scale-to-zero problem. Dev and batch inference workloads should release GPUs when idle, but standard HPA can't scale GPU pods to zero.

## 3. Why Native Integration Fails

I tried putting GPU support inside KEDA core first. It doesn't work, for three reasons:

### 3.1 CGO Constraint

NVIDIA's Go bindings ([go-nvml](https://github.com/NVIDIA/go-nvml)) call into `libnvidia-ml.so` via cgo. KEDA builds its operator with `CGO_ENABLED=0` for portability — every binary is a static Linux ELF. Adding a cgo dependency would break KEDA's build and release pipeline.

This isn't going to change. KEDA's build pipeline and NVIDIA's library have incompatible requirements.

### 3.2 Node-Level Hardware Access

NVML reads GPU state through `/dev/nvidiactl` and `/dev/nvidia0..N`. These device files are only available on the physical GPU node. The KEDA operator runs as a single centralized Deployment — it has no access to GPU devices on worker nodes.

You need a DaemonSet for this. Each instance runs on a GPU node, mounts the NVIDIA device files, and reads metrics locally. There's no way around it.

### 3.3 Independent Release Cycle

GPU stuff moves fast — new architectures (Hopper, Blackwell), new inference servers (vLLM, SGLang), new metrics (MIG partitions, NVLink bandwidth). KEDA releases need to coordinate across 50+ scalers. We'd be waiting months to ship a one-line GPU fix.

This design was discussed and documented in [KEDA issue #7538](https://github.com/kedacore/keda/issues/7538).

## 4. Architecture: Node-Level Telemetry via External gRPC Scalers

### 4.1 Overview

```
GPU Node                                    KEDA Operator
┌──────────────────────────────┐           ┌──────────────────┐
│  DaemonSet: keda-gpu-scaler  │           │                  │
│                              │           │  ExternalScaler  │
│  ┌────────────┐              │  gRPC     │  trigger config  │
│  │ NVML read  │──metrics──►  │──:6000──► │                  │
│  │ on request │              │           │  → HPA decision  │
│  └────────────┘              │           │  → scale up/down │
│       ↕                      │           └──────────────────┘
│  libnvidia-ml.so             │
│  /dev/nvidia0..N             │
└──────────────────────────────┘
```

### 4.2 Data Flow

1. KEDA calls `GetMetrics()` once per `pollingInterval` set on the ScaledObject; the DaemonSet reads NVML at that moment
2. Each cycle reads: SM utilization, memory controller utilization, VRAM used/total, temperature, power draw
3. Nothing is cached or stored; every request is a fresh read
4. KEDA calls `GetMetrics()` over gRPC on the `externalscaler.ExternalScalerServer` interface
5. The scaler returns the requested metric with the aggregation method specified in the ScaledObject
6. KEDA feeds the metric value into HPA for a scale up/down/to-zero decision

### 4.3 gRPC Interface

The scaler implements four methods from KEDA's ExternalScaler protobuf contract:

| Method | Purpose |
|--------|---------|
| `IsActive` | Returns true if any GPU metric exceeds the activation threshold (enables scale-from-zero) |
| `StreamIsActive` | Streaming version for push-based activation |
| `GetMetricSpec` | Returns the metric name and target value for HPA |
| `GetMetrics` | Returns the current GPU metric value |

### 4.4 Why gRPC Over HTTP Metrics

KEDA's external scaler protocol is gRPC, which works well here. Protobuf gives you type safety (no PromQL string parsing). `StreamIsActive` lets KEDA get push notifications for scale-from-zero. And there's no Prometheus in the critical path — just a direct gRPC call.

### 4.5 Security Model

- The DaemonSet needs read-only access to NVIDIA device files — no cluster-wide RBAC
- The gRPC port (6000) is exposed only as a ClusterIP Service — not reachable outside the cluster
- The optional metrics port (9090) can be disabled with `--metrics-port=0`
- No secrets or credentials required
- All NVML calls are read-only (metrics collection, no device configuration)

## 5. Scaling Profiles for AI Workloads

Nobody wants to figure out what "80% GPU utilization" means for their specific workload. For vLLM it's KV cache saturation. For Triton it's SM occupancy across models. Profiles give you sane defaults:

| Profile | Primary Metric | Threshold | Activation | Rationale |
|---------|---------------|-----------|------------|-----------|
| `vllm-inference` | VRAM utilization | 80% | 5% | vLLM pre-allocates KV cache; VRAM pressure = request queue growth |
| `triton-inference` | SM utilization | 75% | 10% | Triton shares GPU across models; SM saturation = throughput degradation |
| `training` | SM utilization | 90% | 0% (no scale-to-zero) | Avoid killing checkpoints; scale only when GPUs are fully saturated |
| `batch` | VRAM utilization | 70% | 1% | Aggressive scale-down; release GPUs quickly for batch inference |

You can override any of these in the ScaledObject metadata — the profiles are just starting points.

## 6. Multi-GPU Aggregation Strategies

When a node has 4 or 8 GPUs, you need to decide how to turn per-GPU metrics into a single number for KEDA:

| Strategy | Behavior | Best For |
|----------|----------|----------|
| **max** (default) | Scale when any GPU hits threshold | Inference — hot GPUs indicate overload |
| **avg** | Scale on average utilization | Training — GPUs should be evenly loaded |
| **min** | Scale when the least-loaded GPU hits threshold | Conservative scaling |
| **sum** | Total utilization across all GPUs | Capacity-based scaling decisions |

For inference, use `max` — one hot GPU means you have a latency problem, even if the others are cool. For training, use `avg` — you want to know if the GPUs are evenly loaded.

## 7. NUMA-Aware GPU Scheduling

keda-gpu-scaler tells you *when* to add pods. But once you scale up, where do the new pods land? That's a scheduling problem, and it matters more than people think.

### 7.1 The NUMA Problem

In a dual-socket server, each GPU is physically wired to one NUMA node via PCIe. If the scheduler puts your pod's CPU on NUMA 0 and the GPU is on NUMA 1, every memory access crosses the inter-socket link (UPI or xGMI). That's an extra 40-100ns per access. For LLM inference and model loading, this adds up fast.

### 7.2 Topology-Aware Placement

I contributed NUMA-aware GPU placement to Volcano (PR #5095). A resource-exporter DaemonSet discovers GPU-to-NUMA mappings at boot and publishes them as node annotations:

```
volcano.sh/gpu-topology: '{"gpus":[{"id":0,"numa":0},{"id":1,"numa":0},{"id":2,"numa":1},{"id":3,"numa":1}]}'
```

The scheduler reads this and co-locates CPU and GPU on the same NUMA node.

### 7.3 Combined Architecture

Run both together and you get:

1. keda-gpu-scaler watches VRAM/SM utilization and tells KEDA to add pods
2. Volcano places the new pods on NUMA nodes that match available GPUs

Scaling + placement, handled.

## 8. Evaluation

### 8.1 Metric Latency

| Approach | Metric Latency | Components |
|----------|---------------|------------|
| DCGM → Prometheus → HPA | 15-30 seconds | 5 |
| DCGM → Prometheus → KEDA | 10-20 seconds | 4 |
| **keda-gpu-scaler (direct NVML)** | **KEDA `pollingInterval` only** | **2** (DaemonSet + KEDA) |

### 8.2 Production Observations

I've been running keda-gpu-scaler on a 4-node GPU cluster (8x A100 80GB per node, EKS) serving LLM inference via vLLM (Llama 3 70B, 4-bit quantized). Some things I observed:

- **VRAM is the right signal for vLLM.** SM utilization stays flat around 60-70% even under heavy load because vLLM batches requests. VRAM pressure tracks actual request queue depth much more closely — once KV cache fills past ~80%, latency spikes within seconds.
- **Scale-to-zero saves real money.** Dev/staging inference endpoints sit idle 18+ hours a day. Releasing those GPUs when nobody's hitting the endpoint cut our GPU spend on non-prod by roughly 60%.
- **KEDA's polling interval is the only delay.** With the Prometheus pipeline, a traffic burst can go unseen for 15-30 seconds of scrape and adapter delay before the first new pod even starts scheduling. With direct NVML, KEDA sees the spike on its next poll and the HPA reacts on the next reconciliation.
- **Multi-GPU aggregation choice is workload-dependent.** For tensor-parallel vLLM (spreading one model across 4 GPUs), `avg` works because all GPUs load evenly. For running multiple smaller models on the same node, `max` catches the hot GPU faster.

These are observations from one deployment, not a controlled benchmark. Your numbers will vary depending on model size, request patterns, and node configuration.

### 8.3 Scale-to-Zero

Standard HPA can't scale GPU pods to zero — GPU extended resources are binary. keda-gpu-scaler's `IsActive` / `StreamIsActive` gRPC methods give KEDA the activation signal it needs. When all GPUs are idle below the activation threshold, KEDA scales to zero. When traffic arrives, `StreamIsActive` fires and KEDA scales back up.

### 8.4 Operational Overhead

| Metric | DCGM + Prometheus Pipeline | keda-gpu-scaler |
|--------|---------------------------|------------------|
| Pods per GPU node | 2 (DCGM exporter + Prometheus) | 1 (DaemonSet) |
| Cluster-wide pods | 3+ (Prometheus server, adapter, alertmanager) | 0 additional |
| Configuration | PromQL queries, scrape configs, adapter rules | ScaledObject YAML |
| Storage | Prometheus TSDB (persistent volume) | None (in-memory) |

## 9. Related Work

| Project | Approach | Limitation |
|---------|----------|------------|
| **DCGM Exporter** | Prometheus metrics from NVIDIA DCGM | Requires full Prometheus pipeline; no direct KEDA integration |
| **gpu-feature-discovery** | Labels nodes with GPU properties | Static labels, no runtime metrics |
| **HAMi** | GPU sharing and virtualization | Focuses on GPU partitioning, not autoscaling |
| **KubeAI** | AI workload management on Kubernetes | Higher-level abstraction; uses standard HPA |
| **Karpenter** | Node-level autoscaling | Scales nodes, not pods; no GPU metric awareness |

keda-gpu-scaler doesn't replace any of these — you probably still want DCGM Exporter for Grafana dashboards. It just takes the scaling decision off the monitoring pipeline and makes it a direct hardware read.

## 10. Future Directions

- **AMD ROCm support**: Same DaemonSet pattern with `rocm-smi` library for AMD Instinct GPUs
- **MIG metrics**: NVIDIA Multi-Instance GPU partitions each have independent utilization metrics
- **NVLink topology**: Prefer scaling on nodes with direct GPU-to-GPU interconnect for multi-GPU inference
- **vLLM queue depth**: Read pending request count directly from vLLM's engine API for predictive scaling
- **Intel Gaudi support**: Extend to Intel's AI accelerators using Habana Management Library
- **Federated scaling**: Coordinate scaling decisions across multiple GPU clusters for global load balancing

## 11. Conclusion

Kubernetes doesn't know what's happening on your GPUs, and the existing workarounds add too much latency and too many moving parts.

The DaemonSet + gRPC pattern works because it respects the constraints: NVML needs CGO, GPU devices are node-local, and GPU tooling moves faster than KEDA's release cycle. Direct NVML reads remove the 15-30 seconds of scrape and adapter delay you get with the Prometheus pipeline; what remains is KEDA's own polling interval.

I've been running this in production and the architecture holds up. The code is at [keda-gpu-scaler](https://github.com/pmady/keda-gpu-scaler) — contributions welcome.

## 12. References

1. KEDA - Kubernetes Event-Driven Autoscaling. https://keda.sh
2. NVIDIA Management Library (NVML). https://developer.nvidia.com/management-library-nvml
3. KEDA External Scaler specification. https://keda.sh/docs/latest/concepts/external-scalers/
4. KEDA Issue #7538 — GPU Scaler Discussion. https://github.com/kedacore/keda/issues/7538
5. keda-gpu-scaler. https://github.com/pmady/keda-gpu-scaler
6. CNCF Blog: GPU Autoscaling on Kubernetes with KEDA. https://www.cncf.io/blog/2026/05/27/gpu-autoscaling-on-kubernetes-with-keda-building-an-external-scaler/
7. Volcano Batch Scheduler. https://volcano.sh
8. Volcano PR #5095 — GPU NUMA-Aware Scheduling. https://github.com/volcano-sh/volcano/pull/5095
9. NVIDIA DCGM Exporter. https://github.com/NVIDIA/dcgm-exporter
10. HAMi - Heterogeneous AI Computing Virtualization Middleware. https://github.com/Project-HAMi/HAMi
11. Kubernetes WG Workload-Aware Scheduling. https://github.com/kubernetes/community/pull/8970
12. go-nvml - NVIDIA Management Library Bindings for Go. https://github.com/NVIDIA/go-nvml
13. AI Business: OpenAI vs. Anthropic vs. Google: But the Model Isn't the Point (Media Mention). https://aibusiness.com/generative-ai/openai-vs-anthropic-vs-google-model-isn-t-the-point
