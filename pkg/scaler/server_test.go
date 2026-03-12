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
	"testing"

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
