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

package scaler

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"google.golang.org/grpc"

	pb "github.com/pmady/keda-gpu-scaler/pkg/externalscaler"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"github.com/pmady/keda-gpu-scaler/pkg/profiles"
)

func TestParseMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     scalerConfig
		wantErr  bool
	}{
		{
			name:     "defaults with no metadata",
			metadata: map[string]string{},
			want: scalerConfig{
				metricName:          "keda_gpu_metric",
				metricType:          profiles.MetricGPUUtilization,
				targetValue:         80,
				activationThreshold: 0,
				gpuIndex:            -1,
				aggregation:         "max",
				pollIntervalSeconds: 10,
			},
		},
		{
			name: "vllm-inference profile",
			metadata: map[string]string{
				"profile": "vllm-inference",
			},
			want: scalerConfig{
				metricName:          "keda_gpu_vllm_inference",
				metricType:          profiles.MetricMemoryUsedPercent,
				targetValue:         80,
				activationThreshold: 5,
				gpuIndex:            -1,
				aggregation:         "max",
				pollIntervalSeconds: 10,
			},
		},
		{
			name: "triton-inference profile",
			metadata: map[string]string{
				"profile": "triton-inference",
			},
			want: scalerConfig{
				metricName:          "keda_gpu_triton_inference",
				metricType:          profiles.MetricGPUUtilization,
				targetValue:         75,
				activationThreshold: 10,
				gpuIndex:            -1,
				aggregation:         "max",
				pollIntervalSeconds: 10,
			},
		},
		{
			name: "profile with overrides",
			metadata: map[string]string{
				"profile":             "vllm-inference",
				"targetValue":         "90",
				"activationThreshold": "10",
				"gpuIndex":            "2",
				"aggregation":         "avg",
			},
			want: scalerConfig{
				metricName:          "keda_gpu_vllm_inference",
				metricType:          profiles.MetricMemoryUsedPercent,
				targetValue:         90,
				activationThreshold: 10,
				gpuIndex:            2,
				aggregation:         "avg",
				pollIntervalSeconds: 10,
			},
		},
		{
			name: "custom metric type",
			metadata: map[string]string{
				"metricType":  "memory_used_mib",
				"targetValue": "40000",
			},
			want: scalerConfig{
				metricName:          "keda_gpu_metric",
				metricType:          profiles.MetricMemoryUsedMiB,
				targetValue:         40000,
				activationThreshold: 0,
				gpuIndex:            -1,
				aggregation:         "max",
				pollIntervalSeconds: 10,
			},
		},
		{
			name: "targetGpuUtilization shorthand",
			metadata: map[string]string{
				"targetGpuUtilization": "85",
			},
			want: scalerConfig{
				metricName:          "keda_gpu_metric",
				metricType:          profiles.MetricGPUUtilization,
				targetValue:         85,
				activationThreshold: 0,
				gpuIndex:            -1,
				aggregation:         "max",
				pollIntervalSeconds: 10,
			},
		},
		{
			name: "targetMemoryUtilization shorthand",
			metadata: map[string]string{
				"targetMemoryUtilization": "70",
			},
			want: scalerConfig{
				metricName:          "keda_gpu_metric",
				metricType:          profiles.MetricMemoryUsedPercent,
				targetValue:         70,
				activationThreshold: 0,
				gpuIndex:            -1,
				aggregation:         "max",
				pollIntervalSeconds: 10,
			},
		},
		{
			name: "unknown profile",
			metadata: map[string]string{
				"profile": "nonexistent",
			},
			wantErr: true,
		},
		{
			name: "invalid targetValue",
			metadata: map[string]string{
				"targetValue": "not-a-number",
			},
			wantErr: true,
		},
		{
			name: "invalid gpuIndex",
			metadata: map[string]string{
				"gpuIndex": "abc",
			},
			wantErr: true,
		},
		{
			name: "invalid aggregation",
			metadata: map[string]string{
				"aggregation": "median",
			},
			wantErr: true,
		},
		{
			name: "invalid metricType typo",
			metadata: map[string]string{
				"metricType": "gpu_utilzation",
			},
			wantErr: true,
		},
		{
			name: "valid non-default metricType",
			metadata: map[string]string{
				"metricType": "temperature",
			},
			want: scalerConfig{
				metricName:          "keda_gpu_metric",
				metricType:          profiles.MetricTemperature,
				targetValue:         80,
				activationThreshold: 0,
				gpuIndex:            -1,
				aggregation:         "max",
				pollIntervalSeconds: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMetadata(tt.metadata)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseMetadata() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if got.metricName != tt.want.metricName {
				t.Errorf("metricName = %v, want %v", got.metricName, tt.want.metricName)
			}
			if got.metricType != tt.want.metricType {
				t.Errorf("metricType = %v, want %v", got.metricType, tt.want.metricType)
			}
			if got.targetValue != tt.want.targetValue {
				t.Errorf("targetValue = %v, want %v", got.targetValue, tt.want.targetValue)
			}
			if got.activationThreshold != tt.want.activationThreshold {
				t.Errorf("activationThreshold = %v, want %v", got.activationThreshold, tt.want.activationThreshold)
			}
			if got.gpuIndex != tt.want.gpuIndex {
				t.Errorf("gpuIndex = %v, want %v", got.gpuIndex, tt.want.gpuIndex)
			}
			if got.aggregation != tt.want.aggregation {
				t.Errorf("aggregation = %v, want %v", got.aggregation, tt.want.aggregation)
			}
			if got.pollIntervalSeconds != tt.want.pollIntervalSeconds {
				t.Errorf("pollIntervalSeconds = %v, want %v", got.pollIntervalSeconds, tt.want.pollIntervalSeconds)
			}
		})
	}
}

func TestExtractMetric(t *testing.T) {
	m := gpu.Metrics{
		Index:              0,
		UUID:               "GPU-abc-123",
		Name:               "NVIDIA A100",
		GPUUtilization:     75,
		MemoryUtilization:  60,
		MemoryUsedMiB:      40960,
		MemoryTotalMiB:     81920,
		TemperatureCelsius: 65,
		PowerDrawWatts:     250,
		PowerLimitWatts:    400,
	}

	tests := []struct {
		name       string
		metricType profiles.MetricType
		want       float64
	}{
		{"gpu_utilization", profiles.MetricGPUUtilization, 75},
		{"memory_utilization", profiles.MetricMemoryUtilization, 60},
		{"memory_used_mib", profiles.MetricMemoryUsedMiB, 40960},
		{"memory_used_percent", profiles.MetricMemoryUsedPercent, 50}, // 40960/81920 * 100
		{"temperature", profiles.MetricTemperature, 65},
		{"power_draw", profiles.MetricPowerDraw, 250},
		{"unknown defaults to gpu_util", MetricType("unknown"), 75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMetric(m, tt.metricType)
			if got != tt.want {
				t.Errorf("extractMetric(%v) = %v, want %v", tt.metricType, got, tt.want)
			}
		})
	}
}

func TestExtractMetricZeroMemory(t *testing.T) {
	m := gpu.Metrics{
		MemoryTotalMiB: 0,
		MemoryUsedMiB:  0,
	}
	got := extractMetric(m, profiles.MetricMemoryUsedPercent)
	if got != 0 {
		t.Errorf("extractMetric with zero total memory = %v, want 0", got)
	}
}

func TestAggregate(t *testing.T) {
	values := []float64{10, 20, 30, 40, 50}

	tests := []struct {
		name   string
		method string
		want   float64
	}{
		{"max", "max", 50},
		{"min", "min", 10},
		{"avg", "avg", 30},
		{"sum", "sum", 150},
		{"unknown defaults to first", "unknown", 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := aggregate(values, tt.method)
			if got != tt.want {
				t.Errorf("aggregate(%v) = %v, want %v", tt.method, got, tt.want)
			}
		})
	}
}

func TestAggregateEmpty(t *testing.T) {
	got := aggregate([]float64{}, "max")
	if got != 0 {
		t.Errorf("aggregate(empty) = %v, want 0", got)
	}
}

// MetricType alias for the test that uses a raw string
type MetricType = profiles.MetricType

func newTestScaler(devices []gpu.Metrics) *GPUExternalScaler {
	logger, _ := zap.NewDevelopment()
	return NewGPUExternalScaler(gpu.NewMockCollector(devices), logger)
}

var testDevices = []gpu.Metrics{
	{Index: 0, UUID: "GPU-0", Name: "A100", GPUUtilization: 80, MemoryUtilization: 60, MemoryUsedMiB: 40960, MemoryTotalMiB: 81920, TemperatureCelsius: 65, PowerDrawWatts: 250, PowerLimitWatts: 400, PCIeTxKBps: 8000, PCIeRxKBps: 4000, NVLinkTxMBps: 600, NVLinkRxMBps: 500},
	{Index: 1, UUID: "GPU-1", Name: "A100", GPUUtilization: 30, MemoryUtilization: 20, MemoryUsedMiB: 16384, MemoryTotalMiB: 81920, TemperatureCelsius: 45, PowerDrawWatts: 100, PowerLimitWatts: 400, PCIeTxKBps: 2000, PCIeRxKBps: 1000, NVLinkTxMBps: 200, NVLinkRxMBps: 150},
}

func TestIsActive(t *testing.T) {
	s := newTestScaler(testDevices)

	tests := []struct {
		name     string
		metadata map[string]string
		want     bool
	}{
		{
			name:     "active when max GPU util exceeds threshold",
			metadata: map[string]string{"activationThreshold": "50"},
			want:     true, // max(80,30)=80 > 50
		},
		{
			name:     "inactive when max GPU util below threshold",
			metadata: map[string]string{"activationThreshold": "90"},
			want:     false, // max(80,30)=80 < 90
		},
		{
			name:     "active at zero threshold",
			metadata: map[string]string{"activationThreshold": "0"},
			want:     true, // 80 > 0
		},
		{
			name:     "single GPU active",
			metadata: map[string]string{"gpuIndex": "0", "activationThreshold": "50"},
			want:     true, // GPU 0 = 80 > 50
		},
		{
			name:     "single GPU inactive",
			metadata: map[string]string{"gpuIndex": "1", "activationThreshold": "50"},
			want:     false, // GPU 1 = 30 < 50
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := s.IsActive(context.Background(), &pb.ScaledObjectRef{
				Name:           "test-so",
				ScalerMetadata: tt.metadata,
			})
			if err != nil {
				t.Fatalf("IsActive() error = %v", err)
			}
			if resp.Result != tt.want {
				t.Errorf("IsActive() = %v, want %v", resp.Result, tt.want)
			}
		})
	}
}

func TestGetMetricSpec(t *testing.T) {
	s := newTestScaler(testDevices)

	tests := []struct {
		name           string
		metadata       map[string]string
		wantMetricName string
		wantTarget     float64
	}{
		{
			name:           "defaults",
			metadata:       map[string]string{},
			wantMetricName: "keda_gpu_metric",
			wantTarget:     80,
		},
		{
			name:           "vllm profile",
			metadata:       map[string]string{"profile": "vllm-inference"},
			wantMetricName: "keda_gpu_vllm_inference",
			wantTarget:     80,
		},
		{
			name:           "custom target",
			metadata:       map[string]string{"targetValue": "95", "metricName": "custom_metric"},
			wantMetricName: "custom_metric",
			wantTarget:     95,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := s.GetMetricSpec(context.Background(), &pb.ScaledObjectRef{
				Name:           "test-so",
				ScalerMetadata: tt.metadata,
			})
			if err != nil {
				t.Fatalf("GetMetricSpec() error = %v", err)
			}
			if len(resp.MetricSpecs) != 1 {
				t.Fatalf("expected 1 metric spec, got %d", len(resp.MetricSpecs))
			}
			spec := resp.MetricSpecs[0]
			if spec.MetricName != tt.wantMetricName {
				t.Errorf("MetricName = %v, want %v", spec.MetricName, tt.wantMetricName)
			}
			if spec.TargetSizeFloat != tt.wantTarget {
				t.Errorf("TargetSizeFloat = %v, want %v", spec.TargetSizeFloat, tt.wantTarget)
			}
		})
	}
}

func TestGetMetrics(t *testing.T) {
	s := newTestScaler(testDevices)

	tests := []struct {
		name     string
		metadata map[string]string
		want     float64
	}{
		{
			name:     "max GPU util across all GPUs",
			metadata: map[string]string{},
			want:     80, // max(80, 30)
		},
		{
			name:     "avg GPU util",
			metadata: map[string]string{"aggregation": "avg"},
			want:     55, // (80+30)/2
		},
		{
			name:     "sum GPU util",
			metadata: map[string]string{"aggregation": "sum"},
			want:     110, // 80+30
		},
		{
			name:     "min GPU util",
			metadata: map[string]string{"aggregation": "min"},
			want:     30,
		},
		{
			name:     "single GPU memory percent",
			metadata: map[string]string{"gpuIndex": "0", "metricType": "memory_used_percent"},
			want:     50, // 40960/81920 * 100
		},
		{
			name:     "temperature metric",
			metadata: map[string]string{"metricType": "temperature", "aggregation": "max"},
			want:     65,
		},
		{
			name:     "power draw metric",
			metadata: map[string]string{"metricType": "power_draw", "aggregation": "sum"},
			want:     350, // 250+100
		},
		{
			name:     "pcie tx max across GPUs",
			metadata: map[string]string{"metricType": "pcie_tx_kbps", "aggregation": "max"},
			want:     8000, // max(8000, 2000)
		},
		{
			name:     "pcie rx sum across GPUs",
			metadata: map[string]string{"metricType": "pcie_rx_kbps", "aggregation": "sum"},
			want:     5000, // 4000+1000
		},
		{
			name:     "nvlink tx max across GPUs",
			metadata: map[string]string{"metricType": "nvlink_tx_mbps", "aggregation": "max"},
			want:     600, // max(600, 200)
		},
		{
			name:     "nvlink rx avg across GPUs",
			metadata: map[string]string{"metricType": "nvlink_rx_mbps", "aggregation": "avg"},
			want:     325, // (500+150)/2
		},
		{
			name:     "single GPU pcie tx",
			metadata: map[string]string{"gpuIndex": "0", "metricType": "pcie_tx_kbps"},
			want:     8000,
		},
		{
			name:     "single GPU nvlink tx",
			metadata: map[string]string{"gpuIndex": "1", "metricType": "nvlink_tx_mbps"},
			want:     200,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
				ScaledObjectRef: &pb.ScaledObjectRef{
					Name:           "test-so",
					ScalerMetadata: tt.metadata,
				},
			})
			if err != nil {
				t.Fatalf("GetMetrics() error = %v", err)
			}
			if len(resp.MetricValues) != 1 {
				t.Fatalf("expected 1 metric value, got %d", len(resp.MetricValues))
			}
			if resp.MetricValues[0].MetricValueFloat != tt.want {
				t.Errorf("MetricValueFloat = %v, want %v", resp.MetricValues[0].MetricValueFloat, tt.want)
			}
		})
	}
}

// migTestDevices simulates a single A100 partitioned into two MIG instances.
// Shared physical metrics (temperature, power) are identical across both instances;
// per-instance metrics (utilization, memory) differ.
var migTestDevices = []gpu.Metrics{
	{
		Index:              0,
		UUID:               "MIG-GPU-aaaa/3/0",
		Name:               "MIG 3g.40gb",
		IsMIGInstance:      true,
		ParentIndex:        0,
		MigProfile:         "3g.40gb",
		GPUUtilization:     85,
		MemoryUtilization:  70,
		MemoryUsedMiB:      30720,
		MemoryTotalMiB:     40960,
		TemperatureCelsius: 72,
		PowerDrawWatts:     300,
		PowerLimitWatts:    400,
	},
	{
		Index:              1,
		UUID:               "MIG-GPU-aaaa/4/0",
		Name:               "MIG 3g.40gb",
		IsMIGInstance:      true,
		ParentIndex:        0,
		MigProfile:         "3g.40gb",
		GPUUtilization:     10,
		MemoryUtilization:  5,
		MemoryUsedMiB:      2048,
		MemoryTotalMiB:     40960,
		TemperatureCelsius: 72,
		PowerDrawWatts:     300,
		PowerLimitWatts:    400,
	},
}

func TestGetMetrics_MIGInstances(t *testing.T) {
	s := newTestScaler(migTestDevices)

	tests := []struct {
		name     string
		metadata map[string]string
		want     float64
	}{
		{
			name:     "max GPU util across MIG instances",
			metadata: map[string]string{},
			want:     85, // max(85, 10)
		},
		{
			name:     "avg GPU util across MIG instances",
			metadata: map[string]string{"aggregation": "avg"},
			want:     47.5, // (85+10)/2
		},
		{
			name:     "min GPU util across MIG instances",
			metadata: map[string]string{"aggregation": "min"},
			want:     10,
		},
		{
			name:     "sum memory used across MIG instances",
			metadata: map[string]string{"metricType": "memory_used_mib", "aggregation": "sum"},
			want:     32768, // 30720+2048
		},
		{
			// Shared physical metric: both instances carry the same temperature.
			// Max and min should both return 72.
			name:     "temperature shared across MIG instances",
			metadata: map[string]string{"metricType": "temperature", "aggregation": "max"},
			want:     72,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
				ScaledObjectRef: &pb.ScaledObjectRef{
					Name:           "test-so",
					ScalerMetadata: tt.metadata,
				},
			})
			if err != nil {
				t.Fatalf("GetMetrics() error = %v", err)
			}
			if len(resp.MetricValues) != 1 {
				t.Fatalf("expected 1 metric value, got %d", len(resp.MetricValues))
			}
			if resp.MetricValues[0].MetricValueFloat != tt.want {
				t.Errorf("MetricValueFloat = %v, want %v", resp.MetricValues[0].MetricValueFloat, tt.want)
			}
		})
	}
}

func TestExtractMetricPCIeNVLink(t *testing.T) {
	m := gpu.Metrics{
		PCIeTxKBps:   8000,
		PCIeRxKBps:   4000,
		NVLinkTxMBps: 600,
		NVLinkRxMBps: 500,
	}

	tests := []struct {
		metricType profiles.MetricType
		want       float64
	}{
		{profiles.MetricPCIeTxKBps, 8000},
		{profiles.MetricPCIeRxKBps, 4000},
		{profiles.MetricNVLinkTxMBps, 600},
		{profiles.MetricNVLinkRxMBps, 500},
	}

	for _, tt := range tests {
		got := extractMetric(m, tt.metricType)
		if got != tt.want {
			t.Errorf("extractMetric(%v) = %v, want %v", tt.metricType, got, tt.want)
		}
	}
}

func TestDistributedTrainingProfile(t *testing.T) {
	s := newTestScaler(testDevices)

	// distributed-training profile should scale on NVLink TX, target 800 MB/s
	resp, err := s.GetMetricSpec(context.Background(), &pb.ScaledObjectRef{
		Name:           "test-so",
		ScalerMetadata: map[string]string{"profile": "distributed-training"},
	})
	if err != nil {
		t.Fatalf("GetMetricSpec() error = %v", err)
	}
	if resp.MetricSpecs[0].TargetSizeFloat != 800 {
		t.Errorf("target = %v, want 800", resp.MetricSpecs[0].TargetSizeFloat)
	}

	// IsActive should be true when NVLink TX (max=600) > activationThreshold (100)
	active, err := s.IsActive(context.Background(), &pb.ScaledObjectRef{
		Name:           "test-so",
		ScalerMetadata: map[string]string{"profile": "distributed-training"},
	})
	if err != nil {
		t.Fatalf("IsActive() error = %v", err)
	}
	if !active.Result {
		t.Error("IsActive() = false, want true (NVLink TX 600 > activation 100)")
	}
}

func TestGetMetricsNoDevices(t *testing.T) {
	s := newTestScaler([]gpu.Metrics{})
	_, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name:           "test-so",
			ScalerMetadata: map[string]string{},
		},
	})
	if err == nil {
		t.Error("GetMetrics() with no devices should return error")
	}
}

func TestIsActiveInvalidMetadata(t *testing.T) {
	s := newTestScaler(testDevices)
	_, err := s.IsActive(context.Background(), &pb.ScaledObjectRef{
		Name:           "test-so",
		ScalerMetadata: map[string]string{"profile": "nonexistent"},
	})
	if err == nil {
		t.Error("IsActive() with invalid profile should return error")
	}
}

// Per-GPU metric logs must carry the GPU name (issue #75) so operators on
// mixed-GPU clusters can tell which card a value came from.
func TestGetMetricValueLogsGPUName(t *testing.T) {
	tests := []struct {
		name      string
		metadata  map[string]string
		wantLines int
	}{
		{"all GPUs", map[string]string{}, len(testDevices)},
		{"single GPU", map[string]string{"gpuIndex": "0"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			s := NewGPUExternalScaler(gpu.NewMockCollector(testDevices), zap.New(core))

			if _, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
				ScaledObjectRef: &pb.ScaledObjectRef{Name: "test-so", ScalerMetadata: tt.metadata},
			}); err != nil {
				t.Fatalf("GetMetrics() error = %v", err)
			}

			entries := logs.FilterMessage("collected GPU metric").All()
			if len(entries) != tt.wantLines {
				t.Fatalf("got %d per-GPU log lines, want %d", len(entries), tt.wantLines)
			}
			for _, e := range entries {
				fields := e.ContextMap()
				if _, ok := fields["gpu_index"]; !ok {
					t.Errorf("log entry missing gpu_index field: %v", fields)
				}
				name, ok := fields["gpu_name"]
				if !ok {
					t.Errorf("log entry missing gpu_name field: %v", fields)
				} else if name != "A100" {
					t.Errorf("gpu_name = %v, want A100", name)
				}
			}
		})
	}
}

// --- vLLM queue depth / KV cache tests ---

const fakeVLLMMetrics = `# HELP vllm:num_requests_waiting Number of waiting requests
# TYPE vllm:num_requests_waiting gauge
vllm:num_requests_waiting{model_name="llama-7b"} 8
# HELP vllm:num_requests_running Number of running requests
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running{model_name="llama-7b"} 4
# HELP vllm:gpu_cache_usage_perc KV cache usage
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc 0.72
# HELP vllm:num_requests_swapped swapped
# TYPE vllm:num_requests_swapped gauge
vllm:num_requests_swapped 0
`

func fakeVLLMServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fakeVLLMMetrics))
	}))
}

func TestGetMetrics_VLLMQueueDepth(t *testing.T) {
	ts := fakeVLLMServer()
	defer ts.Close()

	s := NewGPUExternalScaler(gpu.NewMockCollector(testDevices), zap.NewNop())
	resp, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name: "vllm-so",
			ScalerMetadata: map[string]string{
				"metricType":   "vllm_queue_depth",
				"vllmEndpoint": ts.URL,
				"targetValue":  "5",
			},
		},
	})
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	if len(resp.MetricValues) == 0 {
		t.Fatal("GetMetrics() returned no metric values")
	}
	if resp.MetricValues[0].MetricValueFloat != 8 {
		t.Errorf("queue depth = %v, want 8", resp.MetricValues[0].MetricValueFloat)
	}
}

func TestGetMetrics_VLLMKVCacheUsage(t *testing.T) {
	ts := fakeVLLMServer()
	defer ts.Close()

	s := NewGPUExternalScaler(gpu.NewMockCollector(testDevices), zap.NewNop())
	resp, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name: "vllm-kv-so",
			ScalerMetadata: map[string]string{
				"metricType":   "vllm_kv_cache_usage",
				"vllmEndpoint": ts.URL,
				"targetValue":  "80",
			},
		},
	})
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	if len(resp.MetricValues) == 0 {
		t.Fatal("GetMetrics() returned no metric values")
	}
	// 0.72 * 100 = 72
	got := resp.MetricValues[0].MetricValueFloat
	if math.Abs(got-72) > 0.1 {
		t.Errorf("kv_cache_usage = %v, want 72", got)
	}
}

func TestIsActive_VLLMQueueDepth(t *testing.T) {
	ts := fakeVLLMServer()
	defer ts.Close()

	s := NewGPUExternalScaler(gpu.NewMockCollector(testDevices), zap.NewNop())

	// Queue depth is 8, activation threshold is 1 → active
	resp, err := s.IsActive(context.Background(), &pb.ScaledObjectRef{
		Name: "vllm-so",
		ScalerMetadata: map[string]string{
			"metricType":          "vllm_queue_depth",
			"vllmEndpoint":        ts.URL,
			"activationThreshold": "1",
		},
	})
	if err != nil {
		t.Fatalf("IsActive() error = %v", err)
	}
	if !resp.Result {
		t.Error("IsActive() = false, want true (queue depth 8 > threshold 1)")
	}
}

func TestParseMetadata_VLLMMissingEndpoint(t *testing.T) {
	_, err := parseMetadata(map[string]string{
		"metricType": "vllm_queue_depth",
	})
	if err == nil {
		t.Error("parseMetadata() should fail when vllmEndpoint is missing for vLLM metric")
	}
}

func TestParseMetadata_VLLMWithEndpoint(t *testing.T) {
	cfg, err := parseMetadata(map[string]string{
		"metricType":   "vllm_queue_depth",
		"vllmEndpoint": "http://vllm:8000/metrics",
	})
	if err != nil {
		t.Fatalf("parseMetadata() error = %v", err)
	}
	if cfg.metricType != profiles.MetricVLLMQueueDepth {
		t.Errorf("metricType = %v, want %v", cfg.metricType, profiles.MetricVLLMQueueDepth)
	}
	if cfg.vllmEndpoint != "http://vllm:8000/metrics" {
		t.Errorf("vllmEndpoint = %v, want http://vllm:8000/metrics", cfg.vllmEndpoint)
	}
}

func TestGetMetrics_VLLMQueueDepthProfile(t *testing.T) {
	ts := fakeVLLMServer()
	defer ts.Close()

	s := NewGPUExternalScaler(gpu.NewMockCollector(testDevices), zap.NewNop())
	resp, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name: "vllm-profile-so",
			ScalerMetadata: map[string]string{
				"profile":      "vllm-queue-depth",
				"vllmEndpoint": ts.URL,
			},
		},
	})
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	if len(resp.MetricValues) == 0 {
		t.Fatal("GetMetrics() returned no metric values")
	}
	if resp.MetricValues[0].MetricName != "keda_gpu_vllm_queue_depth" {
		t.Errorf("metricName = %v, want keda_gpu_vllm_queue_depth", resp.MetricValues[0].MetricName)
	}
	if resp.MetricValues[0].MetricValueFloat != 8 {
		t.Errorf("queue depth = %v, want 8", resp.MetricValues[0].MetricValueFloat)
	}
}

// mockStreamIsActive implements pb.ExternalScaler_StreamIsActiveServer for
// testing StreamIsActive: Send pushes onto sent, Context returns ctx, and the
// remaining ServerStream methods are inherited (unused by StreamIsActive).
type mockStreamIsActive struct {
	grpc.ServerStream
	ctx  context.Context
	sent chan *pb.IsActiveResponse
}

func (m *mockStreamIsActive) Context() context.Context { return m.ctx }

func (m *mockStreamIsActive) Send(resp *pb.IsActiveResponse) error {
	select {
	case m.sent <- resp:
	case <-m.ctx.Done():
	}
	return nil
}

func TestStreamIsActive_InvalidMetadata(t *testing.T) {
	s := newTestScaler(testDevices)
	stream := &mockStreamIsActive{ctx: context.Background(), sent: make(chan *pb.IsActiveResponse, 1)}
	err := s.StreamIsActive(
		&pb.ScaledObjectRef{ScalerMetadata: map[string]string{"profile": "nonexistent"}},
		stream,
	)
	if err == nil {
		t.Error("StreamIsActive() with invalid metadata should return an error")
	}
}

func TestStreamIsActive_ContextCancelled(t *testing.T) {
	s := newTestScaler(testDevices)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before the call, so the stream loop exits at once
	stream := &mockStreamIsActive{ctx: ctx, sent: make(chan *pb.IsActiveResponse, 1)}

	done := make(chan error, 1)
	go func() {
		done <- s.StreamIsActive(&pb.ScaledObjectRef{ScalerMetadata: map[string]string{}}, stream)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("StreamIsActive() = %v, want nil on cancelled context", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StreamIsActive() did not return after context cancellation")
	}
}

func TestStreamIsActive_SendsOnTick(t *testing.T) {
	s := newTestScaler(testDevices)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := &mockStreamIsActive{ctx: ctx, sent: make(chan *pb.IsActiveResponse, 4)}

	// max GPU util across testDevices is 80; threshold 50 => active. Tick every 1s.
	md := map[string]string{"activationThreshold": "50", "pollIntervalSeconds": "1"}
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.StreamIsActive(&pb.ScaledObjectRef{ScalerMetadata: md}, stream)
	}()

	select {
	case resp := <-stream.sent:
		if !resp.Result {
			t.Error("StreamIsActive() sent Result=false, want true (max util 80 > threshold 50)")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StreamIsActive() did not send a result within 3s")
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("StreamIsActive() = %v, want nil after cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StreamIsActive() did not return after cancel")
	}
}

func TestGetMetricSpecInvalidMetadata(t *testing.T) {
	s := newTestScaler(testDevices)
	_, err := s.GetMetricSpec(context.Background(), &pb.ScaledObjectRef{
		ScalerMetadata: map[string]string{"profile": "nonexistent"},
	})
	if err == nil {
		t.Error("GetMetricSpec() with invalid metadata should return an error")
	}
}

func TestGetMetricsInvalidMetadata(t *testing.T) {
	s := newTestScaler(testDevices)
	_, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{ScalerMetadata: map[string]string{"aggregation": "median"}},
	})
	if err == nil {
		t.Error("GetMetrics() with invalid metadata should return an error")
	}
}

// A nil metadata map must be handled like an empty one (all defaults), not panic.
func TestIsActiveNilMetadata(t *testing.T) {
	s := newTestScaler(testDevices)
	resp, err := s.IsActive(context.Background(), &pb.ScaledObjectRef{ScalerMetadata: nil})
	if err != nil {
		t.Fatalf("IsActive() with nil metadata error = %v", err)
	}
	// defaults: activationThreshold 0, max util 80 > 0 => active.
	if !resp.Result {
		t.Error("IsActive() with nil metadata = false, want true (default threshold 0, util 80)")
	}
}

// Each numeric shorthand must reject a non-numeric value.
func TestParseMetadataInvalidShorthands(t *testing.T) {
	for _, key := range []string{"targetValue", "targetGpuUtilization", "targetMemoryUtilization", "activationThreshold"} {
		t.Run(key, func(t *testing.T) {
			if _, err := parseMetadata(map[string]string{key: "not-a-number"}); err == nil {
				t.Errorf("parseMetadata(%s=not-a-number) should return an error", key)
			}
		})
	}
}

// An out-of-range gpuIndex makes the (mock) collector error, which IsActive and
// GetMetrics must propagate rather than swallow.
func TestCollectorErrorPropagates(t *testing.T) {
	s := newTestScaler(testDevices)
	md := map[string]string{"gpuIndex": "99"} // only indices 0,1 exist

	if _, err := s.IsActive(context.Background(), &pb.ScaledObjectRef{ScalerMetadata: md}); err == nil {
		t.Error("IsActive() with out-of-range gpuIndex should return an error")
	}
	if _, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{ScalerMetadata: md},
	}); err == nil {
		t.Error("GetMetrics() with out-of-range gpuIndex should return an error")
	}
}

// errSendStream always fails Send, to exercise StreamIsActive's send-error path.
type errSendStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (m *errSendStream) Context() context.Context        { return m.ctx }
func (m *errSendStream) Send(*pb.IsActiveResponse) error { return errors.New("send failed") }

func TestStreamIsActive_SendError(t *testing.T) {
	s := newTestScaler(testDevices)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.StreamIsActive(&pb.ScaledObjectRef{
			ScalerMetadata: map[string]string{"pollIntervalSeconds": "1"},
		}, &errSendStream{ctx: ctx})
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Error("StreamIsActive() should return the Send error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("StreamIsActive() did not return after a Send error")
	}
}

// When collection fails mid-stream, StreamIsActive must log and keep streaming
// (no Send, no return) until the context is cancelled.
func TestStreamIsActive_CollectorErrorContinues(t *testing.T) {
	s := newTestScaler([]gpu.Metrics{}) // no devices => getMetricValue errors each tick
	ctx, cancel := context.WithCancel(context.Background())
	stream := &mockStreamIsActive{ctx: ctx, sent: make(chan *pb.IsActiveResponse, 1)}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.StreamIsActive(&pb.ScaledObjectRef{
			ScalerMetadata: map[string]string{"pollIntervalSeconds": "1"},
		}, stream)
	}()

	select {
	case <-stream.sent:
		t.Error("StreamIsActive() sent a result despite a collection error")
	case <-time.After(1500 * time.Millisecond):
		// expected: errored, logged, continued — nothing sent
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("StreamIsActive() = %v, want nil after cancel", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("StreamIsActive() did not return after cancel")
	}
}

func TestGetVLLMClientCached(t *testing.T) {
	s := NewGPUExternalScaler(gpu.NewMockCollector(testDevices), zap.NewNop())

	c1 := s.getVLLMClient("http://vllm-a.invalid:8000")
	if c2 := s.getVLLMClient("http://vllm-a.invalid:8000"); c1 != c2 {
		t.Error("getVLLMClient() should return the cached client for the same endpoint")
	}
	if c3 := s.getVLLMClient("http://vllm-b.invalid:8000"); c1 == c3 {
		t.Error("getVLLMClient() should return distinct clients for different endpoints")
	}
}

// A vLLM endpoint that cannot be scraped must surface as an error.
func TestGetMetricsVLLMScrapeError(t *testing.T) {
	ts := fakeVLLMServer()
	url := ts.URL
	ts.Close() // close so the endpoint is unreachable

	s := NewGPUExternalScaler(gpu.NewMockCollector(testDevices), zap.NewNop())
	_, err := s.GetMetrics(context.Background(), &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			ScalerMetadata: map[string]string{"profile": "vllm-queue-depth", "vllmEndpoint": url},
		},
	})
	if err == nil {
		t.Error("GetMetrics() with an unreachable vLLM endpoint should return an error")
	}
}
