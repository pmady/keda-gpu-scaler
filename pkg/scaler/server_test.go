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
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

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
