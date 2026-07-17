/*
Copyright 2026 The keda-gpu-scaler Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package profiles

// Profile defines a pre-built scaling configuration for a specific workload type.
type Profile struct {
	Name               string
	MetricName         string
	Description        string
	TargetValue        float64
	ActivationValue    float64
	MetricType         MetricType
	CooldownSeconds    int
	ScaleUpStabilize   int
	ScaleDownStabilize int
}

// MetricType represents the GPU metric to use for scaling decisions.
type MetricType string

const (
	MetricGPUUtilization    MetricType = "gpu_utilization"
	MetricMemoryUtilization MetricType = "memory_utilization"
	MetricMemoryUsedMiB     MetricType = "memory_used_mib"
	MetricMemoryUsedPercent MetricType = "memory_used_percent"
	MetricTemperature       MetricType = "temperature"
	MetricPowerDraw         MetricType = "power_draw"
	MetricPCIeTxKBps        MetricType = "pcie_tx_kbps"
	MetricPCIeRxKBps        MetricType = "pcie_rx_kbps"
	MetricNVLinkTxMBps      MetricType = "nvlink_tx_mbps"
	MetricNVLinkRxMBps      MetricType = "nvlink_rx_mbps"

	// vLLM engine metrics — scraped from the vLLM /metrics endpoint, not NVML.
	MetricVLLMQueueDepth   MetricType = "vllm_queue_depth"
	MetricVLLMKVCacheUsage MetricType = "vllm_kv_cache_usage"

	// Triton engine metrics — scraped from Triton's own /metrics endpoint,
	// not NVML. Both are derived from Triton's cumulative counters by diffing
	// two consecutive scrapes (see pkg/triton), so they only become
	// meaningful after a scaler has polled the same tritonEndpoint at least
	// twice.
	MetricTritonQueueWaitMs MetricType = "triton_queue_wait_ms"
	MetricTritonRequestRate MetricType = "triton_request_rate"
)

// AllMetricTypes returns every supported MetricType, in a stable order suitable
// for inclusion in user-facing error messages.
func AllMetricTypes() []MetricType {
	return []MetricType{
		MetricGPUUtilization,
		MetricMemoryUtilization,
		MetricMemoryUsedMiB,
		MetricMemoryUsedPercent,
		MetricTemperature,
		MetricPowerDraw,
		MetricPCIeTxKBps,
		MetricPCIeRxKBps,
		MetricNVLinkTxMBps,
		MetricNVLinkRxMBps,
		MetricVLLMQueueDepth,
		MetricVLLMKVCacheUsage,
		MetricTritonQueueWaitMs,
		MetricTritonRequestRate,
	}
}

// IsVLLMMetric reports whether t requires the vLLM engine endpoint rather than NVML.
func IsVLLMMetric(t MetricType) bool {
	return t == MetricVLLMQueueDepth || t == MetricVLLMKVCacheUsage
}

// IsTritonMetric reports whether t requires the Triton engine endpoint rather than NVML.
func IsTritonMetric(t MetricType) bool {
	return t == MetricTritonQueueWaitMs || t == MetricTritonRequestRate
}

// ValidMetricType reports whether t is a recognized MetricType.
func ValidMetricType(t MetricType) bool {
	switch t {
	case MetricGPUUtilization, MetricMemoryUtilization, MetricMemoryUsedMiB,
		MetricMemoryUsedPercent, MetricTemperature, MetricPowerDraw,
		MetricPCIeTxKBps, MetricPCIeRxKBps, MetricNVLinkTxMBps, MetricNVLinkRxMBps,
		MetricVLLMQueueDepth, MetricVLLMKVCacheUsage,
		MetricTritonQueueWaitMs, MetricTritonRequestRate:
		return true
	default:
		return false
	}
}

// Built-in profiles for common AI/ML workloads.
var builtinProfiles = map[string]Profile{
	"distributed-training": {
		Name:               "distributed-training",
		MetricName:         "keda_gpu_distributed_training",
		Description:        "Data-parallel training on NVLink systems",
		TargetValue:        50000,
		ActivationValue:    5000,
		MetricType:         MetricNVLinkTxMBps,
		CooldownSeconds:    300,
		ScaleUpStabilize:   60,
		ScaleDownStabilize: 300,
	},
	"vllm-inference": {
		Name:               "vllm-inference",
		MetricName:         "keda_gpu_vllm_inference",
		Description:        "vLLM / LLM serving — memory-based, supports scale-to-zero",
		TargetValue:        80,
		ActivationValue:    5,
		MetricType:         MetricMemoryUsedPercent,
		CooldownSeconds:    60,
		ScaleUpStabilize:   15,
		ScaleDownStabilize: 120,
	},
	"vllm-queue-depth": {
		Name:               "vllm-queue-depth",
		MetricName:         "keda_gpu_vllm_queue_depth",
		Description:        "vLLM queue depth — scale on pending inference requests",
		TargetValue:        5,
		ActivationValue:    1,
		MetricType:         MetricVLLMQueueDepth,
		CooldownSeconds:    30,
		ScaleUpStabilize:   10,
		ScaleDownStabilize: 60,
	},
	"triton-inference": {
		Name:               "triton-inference",
		MetricName:         "keda_gpu_triton_inference",
		Description:        "Triton Inference Server — GPU compute utilization",
		TargetValue:        75,
		ActivationValue:    10,
		MetricType:         MetricGPUUtilization,
		CooldownSeconds:    30,
		ScaleUpStabilize:   10,
		ScaleDownStabilize: 90,
	},
	"triton-queue-wait": {
		Name:               "triton-queue-wait",
		MetricName:         "keda_gpu_triton_queue_wait",
		Description:        "Triton Inference Server — scale on average inference queue wait time via the engine API",
		TargetValue:        50, // milliseconds
		ActivationValue:    5,
		MetricType:         MetricTritonQueueWaitMs,
		CooldownSeconds:    30,
		ScaleUpStabilize:   10,
		ScaleDownStabilize: 60,
	},
	"triton-request-rate": {
		Name:               "triton-request-rate",
		MetricName:         "keda_gpu_triton_request_rate",
		Description:        "Triton Inference Server — scale on inference request rate via the engine API",
		TargetValue:        50, // requests/sec
		ActivationValue:    1,
		MetricType:         MetricTritonRequestRate,
		CooldownSeconds:    30,
		ScaleUpStabilize:   10,
		ScaleDownStabilize: 90,
	},
	"training": {
		Name:               "training",
		MetricName:         "keda_gpu_training",
		Description:        "Training jobs — high GPU util target, no scale-to-zero",
		TargetValue:        90,
		ActivationValue:    0,
		MetricType:         MetricGPUUtilization,
		CooldownSeconds:    300,
		ScaleUpStabilize:   60,
		ScaleDownStabilize: 300,
	},
	"batch": {
		Name:               "batch",
		MetricName:         "keda_gpu_batch",
		Description:        "Batch inference — aggressive scale-down and scale-to-zero",
		TargetValue:        70,
		ActivationValue:    1,
		MetricType:         MetricMemoryUsedPercent,
		CooldownSeconds:    30,
		ScaleUpStabilize:   5,
		ScaleDownStabilize: 60,
	},
	"ollama": {
		Name:               "ollama",
		MetricName:         "keda_gpu_ollama",
		Description:        "Ollama LLM serving — memory-based, supports scale-to-zero",
		TargetValue:        70,
		ActivationValue:    3,
		MetricType:         MetricMemoryUsedPercent,
		CooldownSeconds:    60,
		ScaleUpStabilize:   10,
		ScaleDownStabilize: 120,
	},
	"tgi-inference": {
		Name:               "tgi-inference",
		MetricName:         "keda_gpu_tgi_inference",
		Description:        "HuggingFace TGI serving — memory-based, supports scale-to-zero",
		TargetValue:        75,
		ActivationValue:    5,
		MetricType:         MetricMemoryUsedPercent,
		CooldownSeconds:    45,
		ScaleUpStabilize:   15,
		ScaleDownStabilize: 90,
	},
}

// Get returns a profile by name.
func Get(name string) (Profile, bool) {
	p, ok := builtinProfiles[name]
	return p, ok
}

// List returns all available profile names.
func List() []string {
	names := make([]string, 0, len(builtinProfiles))
	for name := range builtinProfiles {
		names = append(names, name)
	}
	return names
}
