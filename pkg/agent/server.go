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

// Package agent serves this node's GPU metrics over gRPC for a remote
// aggregator. It is the DaemonSet half of the split described in
// docs/design-multi-node-aggregation.md.
package agent

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	nodemetrics "github.com/pmady/keda-gpu-scaler/pkg/nodemetrics"
)

// Server implements the NodeMetrics gRPC service on top of a local collector.
type Server struct {
	nodemetrics.UnimplementedNodeMetricsServer
	collector gpu.MetricsCollector
	node      string
	logger    *zap.Logger
}

// NewServer creates an agent server. node is reported back with every response
// so the aggregator can name the node it could not reach.
func NewServer(collector gpu.MetricsCollector, node string, logger *zap.Logger) *Server {
	return &Server{collector: collector, node: node, logger: logger}
}

// GetDevices returns every GPU on this node with all collected metrics. The
// aggregator asks for the full set once per poll, so the raw values of every
// metric type are in the same response.
func (s *Server) GetDevices(ctx context.Context, _ *nodemetrics.GetDevicesRequest) (*nodemetrics.GetDevicesResponse, error) {
	devices, err := s.collector.CollectAll()
	if err != nil {
		s.logger.Warn("Agent failed to collect GPU metrics", zap.Error(err))
		return nil, status.Errorf(codes.Unavailable, "collect GPU metrics: %v", err)
	}

	out := make([]*nodemetrics.DeviceMetrics, len(devices))
	for i, d := range devices {
		out[i] = toProto(d)
	}

	return &nodemetrics.GetDevicesResponse{Node: s.node, Devices: out}, nil
}

func toProto(m gpu.Metrics) *nodemetrics.DeviceMetrics {
	return &nodemetrics.DeviceMetrics{
		Index:              uint32(m.Index), //nolint:gosec // GPU indices are non-negative.
		Uuid:               m.UUID,
		Name:               m.Name,
		GpuUtilization:     m.GPUUtilization,
		MemoryUtilization:  m.MemoryUtilization,
		MemoryUsedMib:      m.MemoryUsedMiB,
		MemoryTotalMib:     m.MemoryTotalMiB,
		TemperatureCelsius: m.TemperatureCelsius,
		PowerDrawWatts:     m.PowerDrawWatts,
		PowerLimitWatts:    m.PowerLimitWatts,
		PcieTxKbps:         m.PCIeTxKBps,
		PcieRxKbps:         m.PCIeRxKBps,
		NvlinkTxMbps:       m.NVLinkTxMBps,
		NvlinkRxMbps:       m.NVLinkRxMBps,
		MigInstance:        m.IsMIGInstance,
		ParentIndex:        int32(m.ParentIndex), //nolint:gosec // GPU indices are small.
		MigProfile:         m.MigProfile,
	}
}
