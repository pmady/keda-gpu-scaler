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

// Package healthcheck periodically probes NVML (via the GPU collector) and
// reflects its availability in the gRPC Health Checking Protocol
// (google.golang.org/grpc/health) status for the "" (server-wide) service.
package healthcheck

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
)

// DefaultInterval is how often NVML is polled when no interval is configured.
const DefaultInterval = 30 * time.Second

// ServiceName is the gRPC health service name whose status reflects overall
// NVML availability. An empty string is the convention for "the whole server"
// per the gRPC Health Checking Protocol.
const ServiceName = ""

// Checker periodically calls collector.DeviceCount() and updates a gRPC
// health.Server's serving status accordingly: SERVING while NVML responds,
// NOT_SERVING when it errors.
type Checker struct {
	collector gpu.MetricsCollector
	health    *health.Server
	interval  time.Duration
	logger    *zap.Logger

	lastStatus healthpb.HealthCheckResponse_ServingStatus
}

// New returns a Checker that reports status for ServiceName on healthServer.
// If interval is <= 0, DefaultInterval is used.
func New(collector gpu.MetricsCollector, healthServer *health.Server, interval time.Duration, logger *zap.Logger) *Checker {
	if interval <= 0 {
		interval = DefaultInterval
	}
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Checker{
		collector:  collector,
		health:     healthServer,
		interval:   interval,
		logger:     logger,
		lastStatus: healthpb.HealthCheckResponse_UNKNOWN,
	}
}

// Check performs a single NVML probe and updates the health server's status.
// It returns the resulting serving status.
func (c *Checker) Check() healthpb.HealthCheckResponse_ServingStatus {
	status := healthpb.HealthCheckResponse_SERVING
	_, err := c.collector.DeviceCount()
	if err != nil {
		status = healthpb.HealthCheckResponse_NOT_SERVING
	}

	if status != c.lastStatus {
		if status == healthpb.HealthCheckResponse_NOT_SERVING {
			c.logger.Warn("NVML health check failed, reporting NOT_SERVING", zap.Error(err))
		} else {
			c.logger.Info("NVML health check recovered, reporting SERVING")
		}
	}

	c.lastStatus = status
	c.health.SetServingStatus(ServiceName, status)
	return status
}

// Run performs an immediate check and then continues checking every interval
// until ctx is done.
func (c *Checker) Run(ctx context.Context) {
	c.Check()

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.Check()
		}
	}
}
