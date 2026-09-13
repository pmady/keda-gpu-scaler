# Design: Multi-Node GPU Metric Aggregation

Status: Proposed
Tracking issues: [#142](https://github.com/pmady/keda-gpu-scaler/issues/142), [#113](https://github.com/pmady/keda-gpu-scaler/issues/113)
Related: [docs/DESIGN.md](DESIGN.md) (single-node architecture)

## Problem

keda-gpu-scaler runs as a DaemonSet, one pod per GPU node, and each pod only polls
NVML for the GPUs on its own node. A single ClusterIP Service sits in front of all
those pods, so when KEDA opens a gRPC connection it gets load-balanced to one
arbitrary pod and sees just that node's GPUs.

That is fine for a single-node workload but wrong for anything spanning multiple
nodes. Say you have three GPU nodes at 90%, 10%, and 10% utilization. If KEDA hits
the busy node it reports 90% and scales up when it probably shouldn't; if it hits
an idle one it reports 10% and never scales a fleet that is actually saturated.
Worse, the value it gets changes from poll to poll depending on which pod the
Service happened to route to, so scaling behaviour is non-deterministic.

The `aggregation` parameter (`max`/`min`/`avg`/`sum`/`p95`/`p99`) already handles
combining GPUs *within* a node, see
[Multi-GPU Aggregation](DESIGN.md#multi-gpu-aggregation). What's missing is
combining *across* nodes.

## Goals

- Give KEDA one deterministic number for the whole GPU fleet, e.g. average GPU
  utilization across every GPU node.
- Reuse the aggregation strategies we already have (`max`/`min`/`avg`/`sum`/`p95`/`p99`),
  now applied at the cluster level.
- Keep working when some nodes are unreachable. A partial answer beats a failed
  scrape.
- Don't touch the NVML/cgo/device-file constraints. Hardware polling still happens
  on the node, the same as today.

## Non-Goals

- Per-pod GPU attribution through the PodResources API. That's a separate line
  item in [#142](https://github.com/pmady/keda-gpu-scaler/issues/142) and stays
  out of scope here; metrics remain per-device.
- Cross-cluster or federated aggregation.
- Any change to the KEDA-facing ExternalScaler gRPC contract. KEDA keeps calling
  the same four methods; only the answer changes.

## Current architecture

```
GPU Node A            GPU Node B            GPU Node C
┌───────────┐         ┌───────────┐         ┌───────────┐
│ DaemonSet │         │ DaemonSet │         │ DaemonSet │
│  (NVML)   │         │  (NVML)   │         │  (NVML)   │
└─────┬─────┘         └─────┬─────┘         └─────┬─────┘
      └───────────────┬─────┴───────────────┬─────┘
                      │  ClusterIP Service   │
                      │  (random 1 backend)  │
                      └──────────┬───────────┘
                                 │ gRPC :6000
                            ┌────┴─────┐
                            │   KEDA   │  ← sees ONE node
                            └──────────┘
```

## Proposed: agent / aggregator split

Split the binary into two roles, both built from the same image and the same
`MetricsCollector` interface, picked with a `--role` flag:

**Agent** (`--role=agent`) runs as the DaemonSet, one per GPU node. It polls NVML
locally, exactly as the current binary does, and exposes per-node metrics over
gRPC. It no longer serves the KEDA ExternalScaler contract.

**Aggregator** (`--role=aggregator`) runs as a small Deployment with a couple of
replicas for availability. It needs no GPUs and builds with `CGO_ENABLED=0`. It
finds the agents, calls them, combines their numbers at the cluster level, and
serves the KEDA ExternalScaler contract. This is the single endpoint KEDA points
at.

```
GPU Node A            GPU Node B            GPU Node C
┌───────────┐         ┌───────────┐         ┌───────────┐
│  Agent    │         │  Agent    │         │  Agent    │
│  (NVML)   │         │  (NVML)   │         │  (NVML)   │
└─────┬─────┘         └─────┬─────┘         └─────┬─────┘
      │  gRPC (per-node metrics)                  │
      └───────────────┬─────┴───────────────┬─────┘
                      ▼                      ▼
                ┌───────────────────────────────┐
                │        Aggregator (Deploy)     │
                │  discover agents → fan-out →   │
                │  cluster aggregation           │
                │  serves ExternalScaler :6000   │
                └───────────────┬───────────────┘
                                │ gRPC
                           ┌────┴─────┐
                           │   KEDA   │  ← sees WHOLE fleet
                           └──────────┘
```

### Discovering agents

The aggregator finds agents through a headless Service in front of the agent
DaemonSet. A headless Service hands back the A records of every ready agent pod,
so the aggregator can reach all of them instead of a single load-balanced VIP.

It refreshes that endpoint list on the scaler poll interval. There's no leader
election, no gossip, and no shared store; the aggregator stays stateless and
rebuilds its view of the fleet from the endpoints each time. RBAC-wise it needs
`get`/`list`/`watch` on endpoints (or EndpointSlices) in its own namespace. The
agents need nothing new, still just the device files.

### Collecting and combining

On each `GetMetrics`/`IsActive` call, and on the `StreamIsActive` ticker, the
aggregator:

1. Looks up the current set of ready agents.
2. Calls them concurrently with a per-agent timeout (2s by default).
3. Takes each agent's per-node value, or its raw per-GPU values when the strategy
   needs them (see below).
4. Runs the same `aggregate()` function that already lives in
   `pkg/scaler/server.go`, this time over the node values.
5. Returns the single number to KEDA.

There's a subtlety in step 3. Not every strategy survives being applied twice.
`max`, `min`, and `sum` do: the max of the per-node maxes is the global max, and
so on. `avg` and the percentiles don't. Averaging per-node averages is wrong once
nodes have different GPU counts, and a percentile of percentiles doesn't mean
anything. So for `avg`, `p95`, and `p99` the agents send their raw per-GPU values
and the aggregator computes the statistic over the flattened fleet-wide set. For
`max`/`min`/`sum` it can skip that and use the already-aggregated node scalar to
keep the payload small.

### When agents are unreachable

- If an agent times out or errors, it drops out of that cycle. The aggregator
  combines whatever responded and logs which nodes it missed. A partial fleet
  never fails the scrape.
- If discovery comes back with no agents at all (an API-server blip), the
  aggregator serves the last good value for a short staleness window. Past that it
  reports `NOT_SERVING` through the existing health check (`pkg/healthcheck`) so
  KEDA and the HPA hold position instead of scaling on garbage.
- Deployments that don't opt into the split keep working. With `--role` unset the
  binary behaves like it does today, agent and ExternalScaler in one process, so
  existing installs don't change.

## Backward compatibility

The combined single-binary mode stays the default. The split is opt-in through
Helm (`aggregator.enabled=true`). The KEDA gRPC contract doesn't change, so a
ScaledObject only needs to point at the aggregator Service when the split is on.
The `aggregation` field keeps its meaning; in split mode it just applies across
the cluster instead of within one node.

## Alternatives considered

**Elect a leader among the DaemonSet pods and have it aggregate the others.** This
piles a second job onto the DaemonSet, needs leader election, and ties the KEDA
endpoint to a pod that also owns device files. Not worth the coupling.

**Push metrics into a shared ConfigMap or CRD.** A 2s poll loop writing to the API
server is a lot of churn, and reading back stale objects is worse than a live
fan-out.

**Run a gossip sidecar between pods.** Far more distributed-systems machinery than
this needs. The endpoint list from the API server already tells us the fleet.

**Lean on Prometheus and PromQL for the cross-node math.** This brings back the
metrics-pipeline latency and extra components that the project set out to avoid in
the first place (see [DESIGN.md](DESIGN.md#problem-statement)).

## Testing

- Unit tests, no GPU needed: run the aggregator fan-out against several fake
  agents backed by `MockCollector`. Check the two-level math for each strategy,
  especially `avg`, `p95`, and `p99` when nodes have uneven GPU counts.
- Degradation: table-driven cases for timeouts, partial responders, zero agents,
  and the staleness window expiring into `NOT_SERVING`.
- E2E, also no GPU: the current e2e harness already uses the mock collector. Add a
  multi-node case with several agent replicas behind the headless Service and
  assert the cluster value is the same no matter which pod KEDA would have hit.

## Rollout

1. This design doc.
2. Add the `--role` flag and the agent's per-node gRPC service. No default
   behaviour change.
3. Build the aggregator role: discovery, fan-out, two-level aggregation, and the
   degradation handling above.
4. Helm: `aggregator.enabled`, the headless agent Service, the aggregator
   Deployment, and its RBAC.
5. Docs: a configuration section for split mode, and an update to
   [DESIGN.md](DESIGN.md#future-work).

## Acceptance criteria (from #113 / #142)

- [x] Design doc in `docs/` describing the chosen approach
- [ ] Implementation of cross-node metric aggregation
- [ ] Graceful degradation when peers are unreachable
- [ ] Integration tests
