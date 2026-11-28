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

	"github.com/pmady/keda-gpu-scaler/pkg/agent"
	"github.com/pmady/keda-gpu-scaler/pkg/aggregator"
	pb "github.com/pmady/keda-gpu-scaler/pkg/externalscaler"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	nodemetrics "github.com/pmady/keda-gpu-scaler/pkg/nodemetrics"
	"github.com/pmady/keda-gpu-scaler/pkg/scaler"
)

// startAgentServer runs one agent (as the DaemonSet would on a GPU node).
func startAgentServer(t *testing.T, devices []gpu.Metrics) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	srv := grpc.NewServer()
	mock := gpu.NewMockCollector(devices)
	nodemetrics.RegisterNodeMetricsServer(srv, agent.NewServer(mock, lis.Addr().String(), zap.NewNop()))

	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), srv.Stop
}

// startAggregatorServer runs the aggregator role: it discovers the given
// agents, merges their devices, and serves the KEDA ExternalScaler contract.
func startAggregatorServer(t *testing.T, agents ...string) (string, func()) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	logger := zap.NewNop()
	remote := aggregator.New(aggregator.Config{
		Resolve: func(context.Context) ([]string, error) { return agents, nil },
		Timeout: time.Second,
		Logger:  logger,
	})

	srv := grpc.NewServer()
	pb.RegisterExternalScalerServer(srv, scaler.NewGPUExternalScaler(remote, logger))

	go func() { _ = srv.Serve(lis) }()
	return lis.Addr().String(), func() {
		srv.Stop()
		_ = remote.Close()
	}
}

func fetchValue(t *testing.T, client pb.ExternalScalerClient, metadata map[string]string) float64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.GetMetrics(ctx, &pb.GetMetricsRequest{
		ScaledObjectRef: &pb.ScaledObjectRef{Name: "test", Namespace: "default", ScalerMetadata: metadata},
	})
	if err != nil {
		t.Fatalf("GetMetrics() error = %v", err)
	}
	if len(resp.GetMetricValues()) != 1 {
		t.Fatalf("got %d metric values, want 1", len(resp.GetMetricValues()))
	}
	return resp.GetMetricValues()[0].GetMetricValueFloat()
}

// TestClusterAggregationUsesEveryGPU checks the two-level math: the scaler
// aggregates over the flattened fleet, so a node with more GPUs carries
// proportionally more weight than an average of per-node averages would.
func TestClusterAggregationUsesEveryGPU(t *testing.T) {
	busy, stopBusy := startAgentServer(t, []gpu.Metrics{
		{Index: 0, GPUUtilization: 90},
		{Index: 1, GPUUtilization: 90},
	})
	defer stopBusy()
	idle, stopIdle := startAgentServer(t, []gpu.Metrics{
		{Index: 0, GPUUtilization: 0},
	})
	defer stopIdle()

	addr, stopAggregator := startAggregatorServer(t, busy, idle)
	defer stopAggregator()

	conn, client := dialScaler(t, addr)
	defer func() { _ = conn.Close() }()

	// Fleet values are 90, 90, 0: avg = 60. Averaging the two node averages
	// would give 45, which is the bug this design avoids.
	if got := fetchValue(t, client, map[string]string{"aggregation": "avg"}); got != 60 {
		t.Errorf("avg over the fleet = %v, want 60", got)
	}
	if got := fetchValue(t, client, map[string]string{"aggregation": "max"}); got != 90 {
		t.Errorf("max over the fleet = %v, want 90", got)
	}
	if got := fetchValue(t, client, map[string]string{"aggregation": "p95"}); got != 90 {
		t.Errorf("p95 over the fleet = %v, want 90", got)
	}
}

// TestClusterAggregationIsDeterministic guards the original problem: a random
// pod answering each poll made the value jump around.
func TestClusterAggregationIsDeterministic(t *testing.T) {
	busy, stopBusy := startAgentServer(t, []gpu.Metrics{{Index: 0, GPUUtilization: 90}})
	defer stopBusy()
	idle, stopIdle := startAgentServer(t, []gpu.Metrics{{Index: 0, GPUUtilization: 10}})
	defer stopIdle()

	addr, stopAggregator := startAggregatorServer(t, busy, idle)
	defer stopAggregator()

	conn, client := dialScaler(t, addr)
	defer func() { _ = conn.Close() }()

	for i := 0; i < 5; i++ {
		if got := fetchValue(t, client, map[string]string{"aggregation": "max"}); got != 90 {
			t.Fatalf("call %d: max = %v, want 90 on every poll", i, got)
		}
	}
}

// TestClusterAggregationSurvivesAgentLoss checks the degradation rule: one
// unreachable node drops out of the cycle instead of failing the scrape.
func TestClusterAggregationSurvivesAgentLoss(t *testing.T) {
	busy, stopBusy := startAgentServer(t, []gpu.Metrics{{Index: 0, GPUUtilization: 80}})
	defer stopBusy()
	idle, stopIdle := startAgentServer(t, []gpu.Metrics{{Index: 0, GPUUtilization: 20}})

	addr, stopAggregator := startAggregatorServer(t, busy, idle)
	defer stopAggregator()

	conn, client := dialScaler(t, addr)
	defer func() { _ = conn.Close() }()

	if got := fetchValue(t, client, map[string]string{"aggregation": "avg"}); got != 50 {
		t.Fatalf("avg with both agents = %v, want 50", got)
	}

	stopIdle()
	if got := fetchValue(t, client, map[string]string{"aggregation": "avg"}); got != 80 {
		t.Errorf("avg with one agent left = %v, want 80", got)
	}
}
