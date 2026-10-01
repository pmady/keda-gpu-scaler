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

package gpu

import "testing"

func TestNewMockDevices(t *testing.T) {
	devices := NewMockDevices(3, 75)
	if len(devices) != 3 {
		t.Fatalf("expected 3 devices, got %d", len(devices))
	}
	for i, d := range devices {
		if d.Index != i {
			t.Errorf("device %d: Index = %d", i, d.Index)
		}
		if d.UUID == "" || d.Name == "" {
			t.Errorf("device %d: UUID and Name must be set, got %q / %q", i, d.UUID, d.Name)
		}
		if d.GPUUtilization != 75 || d.MemoryUtilization != 75 {
			t.Errorf("device %d: utilization = %d/%d, want 75/75", i, d.GPUUtilization, d.MemoryUtilization)
		}
		if d.MemoryTotalMiB == 0 || d.MemoryUsedMiB > d.MemoryTotalMiB {
			t.Errorf("device %d: memory %d/%d MiB is not sensible", i, d.MemoryUsedMiB, d.MemoryTotalMiB)
		}
		if d.PowerDrawWatts > d.PowerLimitWatts {
			t.Errorf("device %d: power draw %d exceeds limit %d", i, d.PowerDrawWatts, d.PowerLimitWatts)
		}
	}
	// UUIDs must be distinct so CollectByUUID can tell devices apart.
	seen := map[string]bool{}
	for _, d := range devices {
		if seen[d.UUID] {
			t.Errorf("duplicate UUID %q", d.UUID)
		}
		seen[d.UUID] = true
	}
}

func TestNewMockDevicesClampsUtilization(t *testing.T) {
	devices := NewMockDevices(1, 250)
	if got := devices[0].GPUUtilization; got != 100 {
		t.Errorf("utilization should clamp to 100, got %d", got)
	}
	if devices[0].MemoryUsedMiB != devices[0].MemoryTotalMiB {
		t.Errorf("at 100%% utilization memory used (%d) should equal total (%d)",
			devices[0].MemoryUsedMiB, devices[0].MemoryTotalMiB)
	}
}

func TestNewMockDevicesZero(t *testing.T) {
	if got := NewMockDevices(0, 50); len(got) != 0 {
		t.Errorf("expected no devices, got %d", len(got))
	}
}

func TestMockCollectorFromMockDevices(t *testing.T) {
	c := NewMockCollector(NewMockDevices(2, 10))
	n, err := c.DeviceCount()
	if err != nil || n != 2 {
		t.Fatalf("DeviceCount = %d, %v; want 2, nil", n, err)
	}
	if _, err := c.CollectByUUID("GPU-mock-00000001"); err != nil {
		t.Errorf("CollectByUUID: %v", err)
	}
	if _, err := c.CollectDevice(2); err == nil {
		t.Error("CollectDevice(2) should be out of range")
	}
}
