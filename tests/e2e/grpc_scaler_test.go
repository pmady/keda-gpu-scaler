//go:build e2e
// +build e2e

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

package e2e

import (
	"context"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/health"

	pb "github.com/pmady/keda-gpu-scaler/pkg/externalscaler"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"github.com/pmady/keda-gpu-scaler/pkg/scaler"
)

func startTestServer(t *testing.T, devices []gpu.Metrics) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	logger, _ := zap.NewDevelopment()
	mock := gpu.NewMockCollector(devices)
	gpuScaler := scaler.NewGPUExternalScaler(mock, logger)

	srv := grpc.NewServer()
	pb.RegisterExternalScalerServer(srv, gpuScaler)

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(srv, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	go func() {
		if err := srv.Serve(lis); err != nil {
			// server stopped
		}
	}()

	return lis.Addr().String(), func() { srv.GracefulStop() }
}

func dialScaler(t *testing.T, addr string) (*grpc.ClientConn, pb.ExternalScalerClient) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("failed to dial gRPC server at %s: %v", addr, err)
	}
	return conn, pb.NewExternalScalerClient(conn)
}

func TestHealthCheck(t *testing.T) {
	devices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 50, MemoryUsedMiB: 4096, MemoryTotalMiB: 8192},
	}
	addr, cleanup := startTestServer(t, devices)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	healthClient := healthpb.NewHealthClient(conn)
	resp, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	if resp.Status != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("expected SERVING, got %v", resp.Status)
	}
}

func TestIsActive(t *testing.T) {
	tests := []struct {
		name       string
		devices    []gpu.Metrics
		metadata   map[string]string
		wantActive bool
	}{
		{
			name: "active when utilization above default threshold",
			devices: []gpu.Metrics{
				{Index: 0, GPUUtilization: 50, MemoryUsedMiB: 4096, MemoryTotalMiB: 8192},
			},
			metadata:   map[string]string{},
			wantActive: true, // default activationThreshold=0, 50 > 0
		},
		{
			name: "inactive when utilization below activation threshold",
			devices: []gpu.Metrics{
				{Index: 0, GPUUtilization: 5, MemoryUsedMiB: 100, MemoryTotalMiB: 8192},
			},
			metadata:   map[string]string{"activationThreshold": "10"},
			wantActive: false, // 5 < 10
		},
		{
			name: "active with vllm-inference profile above threshold",
			devices: []gpu.Metrics{
				{Index: 0, GPUUtilization: 80, MemoryUsedMiB: 6000, MemoryTotalMiB: 8192},
			},
			metadata:   map[string]string{"profile": "vllm-inference"},
			wantActive: true, // memory_used_percent = 73.2%, activationValue=5
		},
		{
			name: "multi-GPU max aggregation",
			devices: []gpu.Metrics{
				{Index: 0, GPUUtilization: 5, MemoryUsedMiB: 100, MemoryTotalMiB: 8192},
				{Index: 1, GPUUtilization: 90, MemoryUsedMiB: 7000, MemoryTotalMiB: 8192},
			},
			metadata:   map[string]string{"activationThreshold": "50"},
			wantActive: true, // max(5, 90) = 90 > 50
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, cleanup := startTestServer(t, tt.devices)
			defer cleanup()

			conn, client := dialScaler(t, addr)
			defer conn.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.IsActive(ctx, &pb.ScaledObjectRef{
				Name:           "test-scaled-object",
				Namespace:      "default",
				ScalerMetadata: tt.metadata,
			})
			if err != nil {
				t.Fatalf("IsActive failed: %v", err)
			}
			if resp.Result != tt.wantActive {
				t.Errorf("IsActive = %v, want %v", resp.Result, tt.wantActive)
			}
		})
	}
}

func TestGetMetricSpec(t *testing.T) {
	devices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 75},
	}
	addr, cleanup := startTestServer(t, devices)
	defer cleanup()

	conn, client := dialScaler(t, addr)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tests := []struct {
		name           string
		metadata       map[string]string
		wantMetricName string
		wantTarget     float64
	}{
		{
			name:           "default metric spec",
			metadata:       map[string]string{},
			wantMetricName: "keda_gpu_metric",
			wantTarget:     80,
		},
		{
			name:           "vllm-inference profile",
			metadata:       map[string]string{"profile": "vllm-inference"},
			wantMetricName: "keda_gpu_vllm_inference",
			wantTarget:     80,
		},
		{
			name:           "custom target value",
			metadata:       map[string]string{"targetValue": "60"},
			wantMetricName: "keda_gpu_metric",
			wantTarget:     60,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := client.GetMetricSpec(ctx, &pb.ScaledObjectRef{
				Name:           "test-scaled-object",
				Namespace:      "default",
				ScalerMetadata: tt.metadata,
			})
			if err != nil {
				t.Fatalf("GetMetricSpec failed: %v", err)
			}
			if len(resp.MetricSpecs) != 1 {
				t.Fatalf("expected 1 metric spec, got %d", len(resp.MetricSpecs))
			}
			spec := resp.MetricSpecs[0]
			if spec.MetricName != tt.wantMetricName {
				t.Errorf("MetricName = %q, want %q", spec.MetricName, tt.wantMetricName)
			}
			if spec.TargetSizeFloat != tt.wantTarget {
				t.Errorf("TargetSize = %v, want %v", spec.TargetSizeFloat, tt.wantTarget)
			}
		})
	}
}

func TestGetMetrics(t *testing.T) {
	tests := []struct {
		name      string
		devices   []gpu.Metrics
		metadata  map[string]string
		wantValue float64
	}{
		{
			name: "single GPU utilization",
			devices: []gpu.Metrics{
				{Index: 0, GPUUtilization: 75, MemoryUsedMiB: 4096, MemoryTotalMiB: 8192},
			},
			metadata:  map[string]string{},
			wantValue: 75, // default metricType is gpu_utilization
		},
		{
			name: "specific GPU index",
			devices: []gpu.Metrics{
				{Index: 0, GPUUtilization: 30},
				{Index: 1, GPUUtilization: 90},
			},
			metadata:  map[string]string{"gpuIndex": "1"},
			wantValue: 90,
		},
		{
			name: "multi-GPU avg aggregation",
			devices: []gpu.Metrics{
				{Index: 0, GPUUtilization: 60},
				{Index: 1, GPUUtilization: 80},
			},
			metadata:  map[string]string{"aggregation": "avg"},
			wantValue: 70, // (60+80)/2
		},
		{
			name: "memory used percent",
			devices: []gpu.Metrics{
				{Index: 0, MemoryUsedMiB: 6144, MemoryTotalMiB: 8192},
			},
			metadata:  map[string]string{"metricType": "memory_used_percent"},
			wantValue: 75, // 6144/8192 * 100
		},
		{
			name: "temperature metric",
			devices: []gpu.Metrics{
				{Index: 0, TemperatureCelsius: 72},
			},
			metadata:  map[string]string{"metricType": "temperature"},
			wantValue: 72,
		},
		{
			name: "power draw metric",
			devices: []gpu.Metrics{
				{Index: 0, PowerDrawWatts: 250},
			},
			metadata:  map[string]string{"metricType": "power_draw"},
			wantValue: 250,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, cleanup := startTestServer(t, tt.devices)
			defer cleanup()

			conn, client := dialScaler(t, addr)
			defer conn.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.GetMetrics(ctx, &pb.GetMetricsRequest{
				ScaledObjectRef: &pb.ScaledObjectRef{
					Name:           "test-scaled-object",
					Namespace:      "default",
					ScalerMetadata: tt.metadata,
				},
				MetricName: "keda_gpu_metric",
			})
			if err != nil {
				t.Fatalf("GetMetrics failed: %v", err)
			}
			if len(resp.MetricValues) != 1 {
				t.Fatalf("expected 1 metric value, got %d", len(resp.MetricValues))
			}
			got := resp.MetricValues[0].MetricValueFloat
			if got != tt.wantValue {
				t.Errorf("MetricValue = %v, want %v", got, tt.wantValue)
			}
		})
	}
}

// Spin up a hot server, check it reports active + high metric,
// then swap to a cold server and confirm it flips.
func TestScaleOutScaleIn(t *testing.T) {
	highDevices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 95, MemoryUsedMiB: 7500, MemoryTotalMiB: 8192},
		{Index: 1, GPUUtilization: 88, MemoryUsedMiB: 7000, MemoryTotalMiB: 8192},
	}
	addr, cleanup := startTestServer(t, highDevices)

	conn, client := dialScaler(t, addr)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	metadata := map[string]string{
		"activationThreshold": "10",
		"targetValue":         "80",
	}

	activeResp, err := client.IsActive(ctx, &pb.ScaledObjectRef{
		Name:           "vllm-deployment",
		Namespace:      "inference",
		ScalerMetadata: metadata,
	})
	if err != nil {
		t.Fatalf("IsActive (high util) failed: %v", err)
	}
	if !activeResp.Result {
		t.Error("expected IsActive=true during high utilization")
	}

	metricsResp, err := client.GetMetrics(ctx, &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name:           "vllm-deployment",
			Namespace:      "inference",
			ScalerMetadata: metadata,
		},
		MetricName: "keda_gpu_metric",
	})
	if err != nil {
		t.Fatalf("GetMetrics (high util) failed: %v", err)
	}
	highValue := metricsResp.MetricValues[0].MetricValueFloat
	if highValue <= 80 {
		t.Errorf("expected metric > 80 (target) for scale-out, got %v", highValue)
	}
	t.Logf("high phase: metric=%v", highValue)

	cancel()
	conn.Close()
	cleanup()

	// now swap to idle GPUs
	lowDevices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 5, MemoryUsedMiB: 500, MemoryTotalMiB: 8192},
		{Index: 1, GPUUtilization: 3, MemoryUsedMiB: 400, MemoryTotalMiB: 8192},
	}
	addr2, cleanup2 := startTestServer(t, lowDevices)
	defer cleanup2()

	conn2, client2 := dialScaler(t, addr2)
	defer conn2.Close()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	activeResp2, err := client2.IsActive(ctx2, &pb.ScaledObjectRef{
		Name:           "vllm-deployment",
		Namespace:      "inference",
		ScalerMetadata: metadata,
	})
	if err != nil {
		t.Fatalf("IsActive (low util) failed: %v", err)
	}
	if activeResp2.Result {
		t.Error("expected IsActive=false during low utilization")
	}

	metricsResp2, err := client2.GetMetrics(ctx2, &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name:           "vllm-deployment",
			Namespace:      "inference",
			ScalerMetadata: metadata,
		},
		MetricName: "keda_gpu_metric",
	})
	if err != nil {
		t.Fatalf("GetMetrics (low util) failed: %v", err)
	}
	lowValue := metricsResp2.MetricValues[0].MetricValueFloat
	if lowValue >= 80 {
		t.Errorf("expected metric < 80 (target) for scale-in, got %v", lowValue)
	}
	t.Logf("low phase: metric=%v", lowValue)
}

// Smoke-test all profiles: call IsActive, GetMetricSpec, GetMetrics.
func TestAllProfiles(t *testing.T) {
	devices := []gpu.Metrics{
		{
			Index:              0,
			UUID:               "GPU-e2e-test-0",
			Name:               "NVIDIA A100-SXM4-80GB",
			GPUUtilization:     65,
			MemoryUtilization:  70,
			MemoryUsedMiB:      57344,
			MemoryTotalMiB:     81920,
			TemperatureCelsius: 58,
			PowerDrawWatts:     300,
			PowerLimitWatts:    400,
		},
	}

	profileNames := []string{"vllm-inference", "triton-inference", "training", "batch"}

	addr, cleanup := startTestServer(t, devices)
	defer cleanup()

	conn, client := dialScaler(t, addr)
	defer conn.Close()

	for _, profile := range profileNames {
		t.Run(profile, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			metadata := map[string]string{"profile": profile}
			ref := &pb.ScaledObjectRef{
				Name:           "test-" + profile,
				Namespace:      "default",
				ScalerMetadata: metadata,
			}

			_, err := client.IsActive(ctx, ref)
			if err != nil {
				t.Errorf("IsActive failed for profile %s: %v", profile, err)
			}

			specResp, err := client.GetMetricSpec(ctx, ref)
			if err != nil {
				t.Errorf("GetMetricSpec failed for profile %s: %v", profile, err)
			}
			if len(specResp.MetricSpecs) != 1 {
				t.Errorf("expected 1 metric spec for %s, got %d", profile, len(specResp.MetricSpecs))
			}

			metricsResp, err := client.GetMetrics(ctx, &pb.GetMetricsRequest{
				ScaledObjectRef: ref,
				MetricName:      specResp.MetricSpecs[0].MetricName,
			})
			if err != nil {
				t.Errorf("GetMetrics failed for profile %s: %v", profile, err)
			}
			if len(metricsResp.MetricValues) != 1 {
				t.Errorf("expected 1 metric value for %s, got %d", profile, len(metricsResp.MetricValues))
			}

			t.Logf("%s: val=%v target=%v",
				profile,
				metricsResp.MetricValues[0].MetricValueFloat,
				specResp.MetricSpecs[0].TargetSizeFloat,
			)
		})
	}
}

func TestBadMetadata(t *testing.T) {
	devices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 50},
	}
	addr, cleanup := startTestServer(t, devices)
	defer cleanup()

	conn, client := dialScaler(t, addr)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	badCases := []struct {
		name     string
		metadata map[string]string
	}{
		{"bogus profile", map[string]string{"profile": "doesnt-exist"}},
		{"non-numeric targetValue", map[string]string{"targetValue": "abc"}},
		{"non-numeric gpuIndex", map[string]string{"gpuIndex": "xyz"}},
		{"bad aggregation", map[string]string{"aggregation": "median"}},
	}

	for _, tc := range badCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := client.IsActive(ctx, &pb.ScaledObjectRef{
				Name:           "bad",
				Namespace:      "default",
				ScalerMetadata: tc.metadata,
			})
			if err == nil {
				t.Errorf("expected error for metadata %v, got nil", tc.metadata)
			}
		})
	}
}

func TestStreamIsActive(t *testing.T) {
	devices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 60, MemoryUsedMiB: 4096, MemoryTotalMiB: 8192},
	}
	addr, cleanup := startTestServer(t, devices)
	defer cleanup()

	conn, client := dialScaler(t, addr)
	defer conn.Close()

	// short poll so we don't wait forever
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stream, err := client.StreamIsActive(ctx, &pb.ScaledObjectRef{
		Name:      "stream-test",
		Namespace: "default",
		ScalerMetadata: map[string]string{
			"pollIntervalSeconds": "1",
		},
	})
	if err != nil {
		t.Fatalf("StreamIsActive call failed: %v", err)
	}

	// read at least one message
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream recv failed: %v", err)
	}
	// 60 > 0 (default activation), should be active
	if !resp.Result {
		t.Errorf("expected stream to report active, got false")
	}
}

// gpuIndex out of range should error from the mock collector
func TestGpuIndexOutOfRange(t *testing.T) {
	devices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 50},
	}
	addr, cleanup := startTestServer(t, devices)
	defer cleanup()

	conn, client := dialScaler(t, addr)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := client.GetMetrics(ctx, &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name:      "oob-test",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				"gpuIndex": "99",
			},
		},
		MetricName: "keda_gpu_metric",
	})
	if err == nil {
		t.Error("expected error for gpuIndex=99 with 1 device, got nil")
	}
}

// min aggregation across 4 GPUs
func TestAggregationMin(t *testing.T) {
	devices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 80},
		{Index: 1, GPUUtilization: 40},
		{Index: 2, GPUUtilization: 90},
		{Index: 3, GPUUtilization: 55},
	}
	addr, cleanup := startTestServer(t, devices)
	defer cleanup()

	conn, client := dialScaler(t, addr)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetMetrics(ctx, &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name:      "min-test",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				"aggregation": "min",
			},
		},
		MetricName: "keda_gpu_metric",
	})
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}
	got := resp.MetricValues[0].MetricValueFloat
	if got != 40 {
		t.Errorf("min aggregation = %v, want 40", got)
	}
}

// sum aggregation
func TestAggregationSum(t *testing.T) {
	devices := []gpu.Metrics{
		{Index: 0, GPUUtilization: 20},
		{Index: 1, GPUUtilization: 30},
		{Index: 2, GPUUtilization: 50},
	}
	addr, cleanup := startTestServer(t, devices)
	defer cleanup()

	conn, client := dialScaler(t, addr)
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetMetrics(ctx, &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{
			Name:      "sum-test",
			Namespace: "default",
			ScalerMetadata: map[string]string{
				"aggregation": "sum",
			},
		},
		MetricName: "keda_gpu_metric",
	})
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}
	got := resp.MetricValues[0].MetricValueFloat
	if got != 100 {
		t.Errorf("sum aggregation = %v, want 100", got)
	}
}
