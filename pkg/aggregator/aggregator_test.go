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

package aggregator_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/pmady/keda-gpu-scaler/pkg/agent"
	"github.com/pmady/keda-gpu-scaler/pkg/aggregator"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	nodemetrics "github.com/pmady/keda-gpu-scaler/pkg/nodemetrics"
)

// fleet is a set of in-process agents, standing in for the DaemonSet pods a
// headless Service would return. bufconn listeners all report the same address,
// so targets are named here instead.
type fleet struct {
	targets []string
	lis     map[string]*bufconn.Listener
}

func newFleet(t *testing.T, devicesPerAgent ...int) *fleet {
	t.Helper()

	f := &fleet{lis: make(map[string]*bufconn.Listener, len(devicesPerAgent))}
	for i, devices := range devicesPerAgent {
		target := fmt.Sprintf("agent-%d", i)
		lis := bufconn.Listen(1 << 20)

		server := grpc.NewServer()
		collector := gpu.NewMockCollector(gpu.NewMockDevices(devices, 50))
		nodemetrics.RegisterNodeMetricsServer(server, agent.NewServer(collector, target, zap.NewNop()))
		go func() { _ = server.Serve(lis) }()
		t.Cleanup(server.Stop)

		f.targets = append(f.targets, target)
		f.lis[target] = lis
	}
	return f
}

// collector returns a fleet collector over f, using the given resolver.
func (f *fleet) collector(clock *time.Time, resolve func(context.Context) ([]string, error)) *aggregator.Collector {
	dial := func(target string) (nodemetrics.NodeMetricsClient, error) {
		lis, ok := f.lis[target]
		if !ok {
			return nil, errors.New("unknown agent " + target)
		}
		conn, err := grpc.NewClient("passthrough:///"+target,
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		return nodemetrics.NewNodeMetricsClient(conn), nil
	}

	return aggregator.New(aggregator.Config{
		Resolve:   resolve,
		Dial:      dial,
		Timeout:   time.Second,
		Staleness: 30 * time.Second,
		Logger:    zap.NewNop(),
		Now:       func() time.Time { return *clock },
	})
}

func TestCollectAllMergesEveryAgent(t *testing.T) {
	f := newFleet(t, 1, 3)
	clock := time.Now()
	c := f.collector(&clock, fixedResolver(f.targets...))

	devices, err := c.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}
	if len(devices) != 4 {
		t.Fatalf("got %d devices across the fleet, want 4 (1 + 3)", len(devices))
	}

	count, err := c.DeviceCount()
	if err != nil {
		t.Fatalf("DeviceCount() error = %v", err)
	}
	if count != 4 {
		t.Errorf("DeviceCount() = %d, want 4", count)
	}
}

func TestCollectAllSkipsUnreachableAgent(t *testing.T) {
	f := newFleet(t, 2, 2)
	if err := f.lis[f.targets[1]].Close(); err != nil { // this agent no longer answers
		t.Fatalf("failed to close the agent listener: %v", err)
	}

	clock := time.Now()
	c := f.collector(&clock, fixedResolver(f.targets...))

	devices, err := c.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v, want a partial answer", err)
	}
	if len(devices) != 2 {
		t.Errorf("got %d devices, want the 2 from the reachable agent", len(devices))
	}
}

func TestStalenessWindow(t *testing.T) {
	f := newFleet(t, 2)
	clock := time.Now()

	var resolveErr error
	resolve := func(context.Context) ([]string, error) {
		if resolveErr != nil {
			return nil, resolveErr
		}
		return f.targets, nil
	}
	c := f.collector(&clock, resolve)

	if _, err := c.CollectAll(); err != nil {
		t.Fatalf("first CollectAll() error = %v", err)
	}

	// A discovery failure inside the window still answers from the cache, so a
	// blip does not look like an empty fleet to KEDA.
	resolveErr = errors.New("dns down")
	if devices, err := c.CollectAll(); err != nil || len(devices) != 2 {
		t.Fatalf("CollectAll() within staleness = (%d devices, %v), want (2, nil)", len(devices), err)
	}

	// Past the window it reports the failure, so the health check can move to
	// NOT_SERVING and KEDA holds position instead of scaling on stale data.
	clock = clock.Add(31 * time.Second)
	_, err := c.CollectAll()
	if err == nil {
		t.Fatal("CollectAll() past staleness = nil error, want an error")
	}
	if !strings.Contains(err.Error(), "last good answer") {
		t.Errorf("error = %q, want it to mention the last good answer", err)
	}
}

func TestCollectByUUIDCrossesNodesAndIndexIsRejected(t *testing.T) {
	f := newFleet(t, 1, 2)
	clock := time.Now()
	c := f.collector(&clock, fixedResolver(f.targets...))

	devices, err := c.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}
	want := devices[2].UUID // last device of the second agent

	got, err := c.CollectByUUID(want)
	if err != nil {
		t.Fatalf("CollectByUUID() error = %v", err)
	}
	if got.UUID != want {
		t.Errorf("UUID = %q, want %q", got.UUID, want)
	}

	if _, err := c.CollectByUUID("does-not-exist"); err == nil {
		t.Error("CollectByUUID(unknown) = nil error, want a not-found error")
	}

	if _, err := c.CollectDevice(0); err == nil {
		t.Error("CollectDevice() in cluster mode = nil error, want a node-local error")
	}
}

func fixedResolver(targets ...string) func(context.Context) ([]string, error) {
	return func(context.Context) ([]string, error) { return targets, nil }
}
