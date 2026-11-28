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

package agent

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	nodemetrics "github.com/pmady/keda-gpu-scaler/pkg/nodemetrics"
)

func TestGetDevicesReturnsEveryMetricField(t *testing.T) {
	collector := gpu.NewMockCollector(gpu.NewMockDevices(2, 75))
	server := NewServer(collector, "gpu-node-a", zap.NewNop())

	resp, err := server.GetDevices(context.Background(), &nodemetrics.GetDevicesRequest{})
	if err != nil {
		t.Fatalf("GetDevices() error = %v", err)
	}
	if resp.GetNode() != "gpu-node-a" {
		t.Errorf("node = %q, want gpu-node-a", resp.GetNode())
	}
	if len(resp.GetDevices()) != 2 {
		t.Fatalf("got %d devices, want 2", len(resp.GetDevices()))
	}

	got := resp.GetDevices()[0]
	want := collectorDevices(t, collector)[0]
	if got.GetUuid() != want.UUID || got.GetIndex() != uint32(want.Index) {
		t.Errorf("device identity = %q/%d, want %q/%d", got.GetUuid(), got.GetIndex(), want.UUID, want.Index)
	}
	if got.GetGpuUtilization() != want.GPUUtilization {
		t.Errorf("gpu_utilization = %d, want %d", got.GetGpuUtilization(), want.GPUUtilization)
	}
	if got.GetMemoryUsedMib() != want.MemoryUsedMiB || got.GetMemoryTotalMib() != want.MemoryTotalMiB {
		t.Errorf("memory = %d/%d, want %d/%d",
			got.GetMemoryUsedMib(), got.GetMemoryTotalMib(), want.MemoryUsedMiB, want.MemoryTotalMiB)
	}
}

func TestGetDevicesReportsCollectorFailure(t *testing.T) {
	server := NewServer(failingCollector{}, "gpu-node-a", zap.NewNop())

	_, err := server.GetDevices(context.Background(), &nodemetrics.GetDevicesRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("code = %v, want Unavailable", status.Code(err))
	}
}

func collectorDevices(t *testing.T, c gpu.MetricsCollector) []gpu.Metrics {
	t.Helper()
	devices, err := c.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}
	return devices
}

type failingCollector struct{}

func (failingCollector) CollectAll() ([]gpu.Metrics, error) { return nil, errors.New("nvml down") }
func (failingCollector) CollectDevice(int) (gpu.Metrics, error) {
	return gpu.Metrics{}, errors.New("nvml down")
}
func (failingCollector) CollectByUUID(string) (gpu.Metrics, error) {
	return gpu.Metrics{}, errors.New("nvml down")
}
func (failingCollector) DeviceCount() (int, error) { return 0, errors.New("nvml down") }
func (failingCollector) DriverVersion() string     { return "" }
func (failingCollector) Close() error              { return nil }
