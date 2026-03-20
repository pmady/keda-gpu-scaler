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
	"fmt"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"github.com/pmady/keda-gpu-scaler/pkg/profiles"
	pb "github.com/pmady/keda-gpu-scaler/pkg/externalscaler"
)

// GPUExternalScaler implements the KEDA ExternalScaler gRPC interface.
type GPUExternalScaler struct {
	pb.UnimplementedExternalScalerServer
	collector gpu.MetricsCollector
	logger    *zap.Logger
}

// NewGPUExternalScaler creates a new GPU external scaler server.
func NewGPUExternalScaler(collector gpu.MetricsCollector, logger *zap.Logger) *GPUExternalScaler {
	return &GPUExternalScaler{
		collector: collector,
		logger:    logger,
	}
}

// IsActive returns true if there is GPU activity above the activation threshold.
func (s *GPUExternalScaler) IsActive(ctx context.Context, ref *pb.ScaledObjectRef) (*pb.IsActiveResponse, error) {
	cfg, err := parseMetadata(ref.ScalerMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scaler metadata: %w", err)
	}

	value, err := s.getMetricValue(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric value: %w", err)
	}

	active := value > cfg.activationThreshold
	s.logger.Debug("IsActive check",
		zap.String("scaledObject", ref.Name),
		zap.Float64("value", value),
		zap.Float64("activationThreshold", cfg.activationThreshold),
		zap.Bool("active", active),
	)

	return &pb.IsActiveResponse{Result: active}, nil
}

// StreamIsActive streams active status updates at regular intervals.
func (s *GPUExternalScaler) StreamIsActive(ref *pb.ScaledObjectRef, stream pb.ExternalScaler_StreamIsActiveServer) error {
	cfg, err := parseMetadata(ref.ScalerMetadata)
	if err != nil {
		return fmt.Errorf("failed to parse scaler metadata: %w", err)
	}

	ticker := time.NewTicker(time.Duration(cfg.pollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stream.Context().Done():
			return nil
		case <-ticker.C:
			value, err := s.getMetricValue(cfg)
			if err != nil {
				s.logger.Warn("failed to get metric value during stream", zap.Error(err))
				continue
			}

			active := value > cfg.activationThreshold
			if err := stream.Send(&pb.IsActiveResponse{Result: active}); err != nil {
				return err
			}
		}
	}
}

// GetMetricSpec returns the metric specification for HPA construction.
func (s *GPUExternalScaler) GetMetricSpec(ctx context.Context, ref *pb.ScaledObjectRef) (*pb.GetMetricSpecResponse, error) {
	cfg, err := parseMetadata(ref.ScalerMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scaler metadata: %w", err)
	}

	return &pb.GetMetricSpecResponse{
		MetricSpecs: []*pb.MetricSpec{
			{
				MetricName:      cfg.metricName,
				TargetSizeFloat: cfg.targetValue,
			},
		},
	}, nil
}

// GetMetrics returns the current GPU metric values.
func (s *GPUExternalScaler) GetMetrics(ctx context.Context, req *pb.GetMetricsRequest) (*pb.GetMetricsResponse, error) {
	cfg, err := parseMetadata(req.ScaledObjectRef.ScalerMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to parse scaler metadata: %w", err)
	}

	value, err := s.getMetricValue(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric value: %w", err)
	}

	s.logger.Debug("GetMetrics",
		zap.String("scaledObject", req.ScaledObjectRef.Name),
		zap.String("metricName", cfg.metricName),
		zap.Float64("value", value),
	)

	return &pb.GetMetricsResponse{
		MetricValues: []*pb.MetricValue{
			{
				MetricName:       cfg.metricName,
				MetricValueFloat: value,
			},
		},
	}, nil
}

// scalerConfig holds parsed configuration from ScaledObject metadata.
type scalerConfig struct {
	metricName          string
	metricType          profiles.MetricType
	targetValue         float64
	activationThreshold float64
	gpuIndex            int // -1 means aggregate all GPUs
	aggregation         string
	pollIntervalSeconds int
}

func parseMetadata(metadata map[string]string) (scalerConfig, error) {
	cfg := scalerConfig{
		metricName:          "keda_gpu_metric",
		metricType:          profiles.MetricGPUUtilization,
		targetValue:         80,
		activationThreshold: 0,
		gpuIndex:            -1,
		aggregation:         "max",
		pollIntervalSeconds: 10,
	}

	// Check for a pre-built profile first
	if profileName, ok := metadata["profile"]; ok {
		profile, found := profiles.Get(profileName)
		if !found {
			return cfg, fmt.Errorf("unknown profile: %s (available: %v)", profileName, profiles.List())
		}
		cfg.metricName = profile.MetricName
		cfg.metricType = profile.MetricType
		cfg.targetValue = profile.TargetValue
		cfg.activationThreshold = profile.ActivationValue
	}

	// Individual overrides take precedence over profile defaults
	if v, ok := metadata["metricName"]; ok {
		cfg.metricName = v
	}
	if v, ok := metadata["metricType"]; ok {
		cfg.metricType = profiles.MetricType(v)
	}
	if v, ok := metadata["targetValue"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid targetValue %q: %w", v, err)
		}
		cfg.targetValue = f
	}
	if v, ok := metadata["targetGpuUtilization"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid targetGpuUtilization %q: %w", v, err)
		}
		cfg.targetValue = f
		cfg.metricType = profiles.MetricGPUUtilization
	}
	if v, ok := metadata["targetMemoryUtilization"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid targetMemoryUtilization %q: %w", v, err)
		}
		cfg.targetValue = f
		cfg.metricType = profiles.MetricMemoryUsedPercent
	}
	if v, ok := metadata["activationThreshold"]; ok {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return cfg, fmt.Errorf("invalid activationThreshold %q: %w", v, err)
		}
		cfg.activationThreshold = f
	}
	if v, ok := metadata["gpuIndex"]; ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid gpuIndex %q: %w", v, err)
		}
		cfg.gpuIndex = i
	}
	if v, ok := metadata["aggregation"]; ok {
		switch v {
		case "max", "min", "avg", "sum":
			cfg.aggregation = v
		default:
			return cfg, fmt.Errorf("invalid aggregation %q: must be max, min, avg, or sum", v)
		}
	}
	if v, ok := metadata["pollIntervalSeconds"]; ok {
		i, err := strconv.Atoi(v)
		if err != nil {
			return cfg, fmt.Errorf("invalid pollIntervalSeconds %q: %w", v, err)
		}
		cfg.pollIntervalSeconds = i
	}

	return cfg, nil
}

// getMetricValue reads the current GPU metric based on the scaler configuration.
func (s *GPUExternalScaler) getMetricValue(cfg scalerConfig) (float64, error) {
	// Single GPU mode
	if cfg.gpuIndex >= 0 {
		m, err := s.collector.CollectDevice(cfg.gpuIndex)
		if err != nil {
			return 0, err
		}
		return extractMetric(m, cfg.metricType), nil
	}

	// Aggregate across all GPUs
	allMetrics, err := s.collector.CollectAll()
	if err != nil {
		return 0, err
	}
	if len(allMetrics) == 0 {
		return 0, fmt.Errorf("no GPU devices found")
	}

	values := make([]float64, len(allMetrics))
	for i, m := range allMetrics {
		values[i] = extractMetric(m, cfg.metricType)
	}

	return aggregate(values, cfg.aggregation), nil
}

func extractMetric(m gpu.Metrics, metricType profiles.MetricType) float64 {
	switch metricType {
	case profiles.MetricGPUUtilization:
		return float64(m.GPUUtilization)
	case profiles.MetricMemoryUtilization:
		return float64(m.MemoryUtilization)
	case profiles.MetricMemoryUsedMiB:
		return float64(m.MemoryUsedMiB)
	case profiles.MetricMemoryUsedPercent:
		if m.MemoryTotalMiB == 0 {
			return 0
		}
		return float64(m.MemoryUsedMiB) / float64(m.MemoryTotalMiB) * 100
	case profiles.MetricTemperature:
		return float64(m.TemperatureCelsius)
	case profiles.MetricPowerDraw:
		return float64(m.PowerDrawWatts)
	default:
		return float64(m.GPUUtilization)
	}
}

func aggregate(values []float64, method string) float64 {
	if len(values) == 0 {
		return 0
	}

	switch method {
	case "max":
		max := values[0]
		for _, v := range values[1:] {
			if v > max {
				max = v
			}
		}
		return max
	case "min":
		min := values[0]
		for _, v := range values[1:] {
			if v < min {
				min = v
			}
		}
		return min
	case "avg":
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum / float64(len(values))
	case "sum":
		sum := 0.0
		for _, v := range values {
			sum += v
		}
		return sum
	default:
		return values[0]
	}
}
