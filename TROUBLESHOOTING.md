# Troubleshooting

This guide covers common deployment and scaling issues for `keda-gpu-scaler`.
Start with the quick checks, then jump to the symptom that matches what you see.

## Quick checks

Check that the scaler is running on GPU nodes:

```bash
kubectl get pods -n keda -l app.kubernetes.io/name=keda-gpu-scaler -o wide
kubectl logs -n keda -l app.kubernetes.io/name=keda-gpu-scaler --tail=100
```

Check that KEDA can see your `ScaledObject`:

```bash
kubectl get scaledobject -A
kubectl describe scaledobject <name> -n <namespace>
kubectl describe hpa <hpa-name> -n <namespace>
```

KEDA creates and manages the HPA for the scaled workload. Describing the HPA can
show whether KEDA is publishing metrics and what replica decisions Kubernetes is
making.

Check the scaler Service and ports:

```bash
kubectl get svc -n keda keda-gpu-scaler
```

The default gRPC port is `6000`, Prometheus metrics use `9090`, and probes use
`8081`.

## Pod cannot access NVML

**Symptom:** The scaler pod crashes or logs an NVML initialization error such as
`failed to initialize NVML`.

**Cause:** NVML is provided by the NVIDIA driver on the host. The pod must run on
a GPU node and be able to load `libnvidia-ml.so.1` and access NVIDIA device
files. This usually fails when the NVIDIA driver, NVIDIA device plugin, or
container runtime integration is missing.

**Fix:**

1. Confirm the node has a working NVIDIA driver:

   ```bash
   nvidia-smi
   ```

2. Confirm the Kubernetes NVIDIA device plugin is installed and GPU nodes expose
   GPU capacity:

   ```bash
   kubectl describe node <gpu-node-name> | grep -i nvidia
   ```

3. If you deploy with Helm, prefer the NVIDIA runtime class when available:

   ```yaml
   runtimeClassName: nvidia
   ```

4. If your cluster does not provide the `nvidia` runtime class, enable the chart
   host mounts instead and verify the host paths match your node image:

   ```yaml
   nvmlHostMounts:
     enabled: true
     nvidiactl: /dev/nvidiactl
     nvidiaUvm: /dev/nvidia-uvm
     nvmlLib: /usr/lib/x86_64-linux-gnu/libnvidia-ml.so.1
   ```

## ScaledObject is not triggering scale-up

**Symptom:** The workload stays at the minimum replica count even when GPU load
is expected to be high.

**Cause:** KEDA is receiving a metric value below the configured activation or
target value, or the `ScaledObject` is pointing at the wrong metric or scaler
address.

**Fix:**

1. Check the `ScaledObject` metadata:

   ```yaml
   triggers:
     - type: external
       metadata:
         scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
         metricType: "gpu_utilization"
         targetValue: "80"
         activationThreshold: "10"
   ```

2. Use a supported `metricType`, such as `gpu_utilization`,
   `memory_used_percent`, `memory_used_mib`, `power_draw`,
   `nvlink_tx_mbps`, or `nvlink_rx_mbps`.

3. Lower `activationThreshold` if scale-to-zero is not activating when expected.
   Lower `targetValue` if the GPU is busy but still below your configured target.

4. For multi-GPU nodes, check whether `aggregation` matches your intent.
   `max` is the default and scales when any GPU crosses the target. `avg` can
   hide one busy GPU if other GPUs are idle.

## Metrics return zero

**Symptom:** KEDA receives `0`, or the `/metrics` endpoint shows zero GPU usage.

**Cause:** The GPU may actually be idle, the scaler may be reading the wrong GPU
index, or the selected metric may not be available on that hardware.

**Fix:**

1. Compare with host-level telemetry while the workload is running:

   ```bash
   nvidia-smi
   ```

2. If you set `gpuIndex`, confirm the workload uses that GPU. Remove `gpuIndex`
   to aggregate all GPUs on the node:

   ```yaml
   metadata:
     metricType: "gpu_utilization"
     aggregation: "max"
   ```

3. If you scale on memory, use `memory_used_percent` for percentage-based
   thresholds or `memory_used_mib` for absolute MiB thresholds.

4. For vLLM engine metrics such as `vllm_queue_depth` or
   `vllm_kv_cache_usage`, set `vllmEndpoint`; those metrics come from vLLM's
   HTTP metrics endpoint, not NVML.

## gRPC connection refused

**Symptom:** KEDA reports connection failures to the external scaler, or the
`ScaledObject` status shows the scaler cannot be reached.

**Cause:** The `scalerAddress` does not match the Service DNS name or gRPC port,
the Service has no ready endpoints, or the scaler pods are not scheduled on the
expected nodes.

**Fix:**

1. Use the default Service address when the scaler runs in the `keda` namespace:

   ```yaml
   scalerAddress: "keda-gpu-scaler.keda.svc.cluster.local:6000"
   ```

2. Confirm the Service and endpoints exist:

   ```bash
   kubectl get svc,endpoints -n keda keda-gpu-scaler
   ```

3. Confirm pods are ready:

   ```bash
   kubectl get pods -n keda -l app.kubernetes.io/name=keda-gpu-scaler
   ```

4. If you can reach the pod or Service network directly, use `grpcurl` to verify
   the external scaler is responding on the gRPC port:

   ```bash
   grpcurl -plaintext <scaler-ip-or-service>:6000 list
   ```

5. If you changed `grpc.port` in Helm or `--port` in the manifest, update both
   the Service port and every `ScaledObject` `scalerAddress`.

## Permission denied on `/dev/nvidia*`

**Symptom:** The scaler pod starts but logs permission errors for
`/dev/nvidiactl`, `/dev/nvidia-uvm`, or other `/dev/nvidia*` devices.

**Cause:** NVML needs access to host GPU device files. The pod security context,
runtime class, or host mounts do not provide the required permissions.

**Fix:**

1. Keep the default chart security context unless your cluster has an equivalent
   policy:

   ```yaml
   securityContext:
     privileged: true
     runAsUser: 0
   ```

2. If your cluster uses Pod Security Admission, allow this DaemonSet in the
   namespace where it runs or deploy it into a namespace with the required GPU
   device access policy.

3. If you cannot use `runtimeClassName: nvidia`, enable `nvmlHostMounts` and
   verify the configured host paths exist on every GPU node.

## MIG metrics are not appearing

**Symptom:** You expect per-instance MIG metrics, but only physical GPU metrics
appear or no MIG entries are returned.

**Cause:** MIG mode may not be enabled on the GPU, no compute instances may be
configured, or the workload may be targeting a physical GPU index instead of MIG
instance UUIDs.

**Fix:**

1. Check MIG mode and configured instances on the node:

   ```bash
   nvidia-smi -L
   nvidia-smi mig -lgi
   ```

2. Remove `gpuIndex` if you want the scaler to collect all devices. `CollectAll`
   auto-detects MIG-enabled GPUs and returns one metric entry per compute
   instance.

3. In SLURM or Flux environments, make sure `CUDA_VISIBLE_DEVICES` contains the
   assigned MIG UUIDs, for example `MIG-GPU-.../3/0`.

4. Remember that some chip-level metrics, such as temperature and power, are
   read from the parent GPU and shared across MIG instances.
