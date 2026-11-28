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

// Package aggregator collects GPU metrics from the per-node agents and
// presents the whole fleet as one collector, so the existing scaler code
// aggregates across nodes with no change to the KEDA-facing contract.
package aggregator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	nodemetrics "github.com/pmady/keda-gpu-scaler/pkg/nodemetrics"
)

// Defaults match docs/design-multi-node-aggregation.md.
const (
	DefaultTimeout   = 2 * time.Second
	DefaultStaleness = 30 * time.Second
)

// Resolver returns the addresses (host:port) of the agents to query.
type Resolver func(ctx context.Context) ([]string, error)

// Config wires a Collector.
type Config struct {
	// Resolve is required.
	Resolve Resolver
	// Dial is optional; it exists for tests that do not want real connections.
	Dial func(target string) (nodemetrics.NodeMetricsClient, error)
	// Timeout bounds one agent call. Defaults to DefaultTimeout.
	Timeout time.Duration
	// Staleness is how long the last complete answer is served when discovery
	// returns no agents at all. Defaults to DefaultStaleness; 0 disables it.
	Staleness time.Duration
	Logger    *zap.Logger
	Now       func() time.Time
}

// Collector implements gpu.MetricsCollector over the fleet's agents.
type Collector struct {
	cfg Config
	now func() time.Time

	mu         sync.Mutex
	conns      map[string]*grpc.ClientConn
	lastGood   []gpu.Metrics
	lastGoodAt time.Time
}

// New creates a fleet collector.
func New(cfg Config) *Collector {
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.Staleness == 0 {
		cfg.Staleness = DefaultStaleness
	}
	if cfg.Logger == nil {
		cfg.Logger = zap.NewNop()
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Collector{cfg: cfg, now: now, conns: make(map[string]*grpc.ClientConn)}
}

// NewDNSResolver resolves a headless Service name to the addresses of all
// ready agent pods. The Kubernetes DNS server returns one A record per pod.
func NewDNSResolver(service string, port int) Resolver {
	return func(ctx context.Context) ([]string, error) {
		hosts, err := net.DefaultResolver.LookupHost(ctx, service)
		if err != nil {
			return nil, err
		}
		addrs := make([]string, 0, len(hosts))
		for _, host := range hosts {
			addrs = append(addrs, net.JoinHostPort(host, strconv.Itoa(port)))
		}
		return addrs, nil
	}
}

// CollectAll queries every agent concurrently and merges what came back.
// An agent that times out drops out of this cycle; the rest of the fleet is
// still returned.
func (c *Collector) CollectAll() ([]gpu.Metrics, error) {
	ctx := context.Background()
	addrs, err := c.cfg.Resolve(ctx)
	if err != nil || len(addrs) == 0 {
		if err == nil {
			err = errors.New("no agents discovered")
		}
		return c.staleOrError(fmt.Errorf("discover agents: %w", err))
	}

	sort.Strings(addrs)

	type result struct {
		addr    string
		devices []gpu.Metrics
		err     error
	}

	results := make(chan result, len(addrs))
	for _, addr := range addrs {
		go func(addr string) {
			devices, callErr := c.fetch(ctx, addr)
			results <- result{addr: addr, devices: devices, err: callErr}
		}(addr)
	}

	merged := make([]gpu.Metrics, 0, len(addrs))
	var failed []string
	for range addrs {
		res := <-results
		if res.err != nil {
			failed = append(failed, res.addr)
			continue
		}
		merged = append(merged, res.devices...)
	}

	if len(failed) > 0 {
		c.cfg.Logger.Warn("Some agents did not answer; aggregating the rest of the fleet",
			zap.Int("failed", len(failed)),
			zap.Int("answered", len(addrs)-len(failed)),
			zap.Strings("unreachable", failed),
		)
	}

	if len(failed) == len(addrs) {
		return c.staleOrError(fmt.Errorf("no agent answered (%d queried)", len(addrs)))
	}

	c.recordGood(merged)
	return merged, nil
}

// CollectDevice is not supported in cluster mode: a device index identifies a
// card on one node only, so the aggregator cannot tell which node is meant.
// Use CollectByUUID, or aggregate over the fleet.
func (c *Collector) CollectDevice(int) (gpu.Metrics, error) {
	return gpu.Metrics{}, errors.New("gpu index selection is node-local and not available in cluster mode; use gpu_uuid instead")
}

// CollectByUUID finds a device by its UUID, which is unique across the fleet.
func (c *Collector) CollectByUUID(uuid string) (gpu.Metrics, error) {
	devices, err := c.CollectAll()
	if err != nil {
		return gpu.Metrics{}, err
	}
	for _, d := range devices {
		if d.UUID == uuid {
			return d, nil
		}
	}
	return gpu.Metrics{}, fmt.Errorf("GPU with UUID %q not found in the fleet", uuid)
}

// DeviceCount returns the number of GPUs across the responding agents.
func (c *Collector) DeviceCount() (int, error) {
	devices, err := c.CollectAll()
	if err != nil {
		return 0, err
	}
	return len(devices), nil
}

// DriverVersion has no cluster-wide meaning; agents may run different drivers.
func (c *Collector) DriverVersion() string { return "" }

// Close releases the pooled agent connections.
func (c *Collector) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	var errs []error
	for addr, conn := range c.conns {
		errs = append(errs, conn.Close())
		delete(c.conns, addr)
	}
	return errors.Join(errs...)
}

func (c *Collector) fetch(parent context.Context, addr string) ([]gpu.Metrics, error) {
	client, err := c.client(addr)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(parent, c.cfg.Timeout)
	defer cancel()

	resp, err := client.GetDevices(ctx, &nodemetrics.GetDevicesRequest{})
	if err != nil {
		return nil, err
	}

	devices := make([]gpu.Metrics, len(resp.GetDevices()))
	for i, d := range resp.GetDevices() {
		devices[i] = fromProto(d)
	}
	return devices, nil
}

func (c *Collector) client(addr string) (nodemetrics.NodeMetricsClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if conn, ok := c.conns[addr]; ok {
		return nodemetrics.NewNodeMetricsClient(conn), nil
	}

	if c.cfg.Dial != nil {
		return c.cfg.Dial(addr)
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	c.conns[addr] = conn
	return nodemetrics.NewNodeMetricsClient(conn), nil
}

func (c *Collector) recordGood(devices []gpu.Metrics) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastGood = devices
	c.lastGoodAt = c.now()
}

// staleOrError returns the last good answer while it is within the staleness
// window, so a discovery blip does not make KEDA scale on an empty fleet.
func (c *Collector) staleOrError(cause error) ([]gpu.Metrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.lastGood) == 0 || c.cfg.Staleness <= 0 {
		return nil, cause
	}
	age := c.now().Sub(c.lastGoodAt)
	if age > c.cfg.Staleness {
		return nil, fmt.Errorf("%w (last good answer was %s ago)", cause, age.Round(time.Second))
	}

	c.cfg.Logger.Warn("Serving the last good fleet metrics", zap.Duration("age", age), zap.Error(cause))
	return c.lastGood, nil
}

func fromProto(d *nodemetrics.DeviceMetrics) gpu.Metrics {
	return gpu.Metrics{
		Index:              int(d.GetIndex()),
		UUID:               d.GetUuid(),
		Name:               d.GetName(),
		GPUUtilization:     d.GetGpuUtilization(),
		MemoryUtilization:  d.GetMemoryUtilization(),
		MemoryUsedMiB:      d.GetMemoryUsedMib(),
		MemoryTotalMiB:     d.GetMemoryTotalMib(),
		TemperatureCelsius: d.GetTemperatureCelsius(),
		PowerDrawWatts:     d.GetPowerDrawWatts(),
		PowerLimitWatts:    d.GetPowerLimitWatts(),
		PCIeTxKBps:         d.GetPcieTxKbps(),
		PCIeRxKBps:         d.GetPcieRxKbps(),
		NVLinkTxMBps:       d.GetNvlinkTxMbps(),
		NVLinkRxMBps:       d.GetNvlinkRxMbps(),
		IsMIGInstance:      d.GetMigInstance(),
		ParentIndex:        int(d.GetParentIndex()),
		MigProfile:         d.GetMigProfile(),
	}
}
