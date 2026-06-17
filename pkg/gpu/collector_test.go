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

import (
	"testing"
)

var twoGPUs = []Metrics{
	{
		Index:              0,
		UUID:               "GPU-aaaa-1111",
		Name:               "NVIDIA A100-SXM4-80GB",
		GPUUtilization:     85,
		MemoryUtilization:  70,
		MemoryUsedMiB:      57344,
		MemoryTotalMiB:     81920,
		TemperatureCelsius: 72,
		PowerDrawWatts:     300,
		PowerLimitWatts:    400,
	},
	{
		Index:              1,
		UUID:               "GPU-bbbb-2222",
		Name:               "NVIDIA A100-SXM4-80GB",
		GPUUtilization:     20,
		MemoryUtilization:  15,
		MemoryUsedMiB:      12288,
		MemoryTotalMiB:     81920,
		TemperatureCelsius: 38,
		PowerDrawWatts:     75,
		PowerLimitWatts:    400,
	},
}

func TestMockCollectorCollectAll(t *testing.T) {
	c := NewMockCollector(twoGPUs)
	got, err := c.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("CollectAll() returned %d devices, want 2", len(got))
	}
	if got[0].UUID != "GPU-aaaa-1111" {
		t.Errorf("device 0 UUID = %v, want GPU-aaaa-1111", got[0].UUID)
	}
	if got[1].GPUUtilization != 20 {
		t.Errorf("device 1 GPUUtilization = %v, want 20", got[1].GPUUtilization)
	}
}

func TestMockCollectorCollectDevice(t *testing.T) {
	c := NewMockCollector(twoGPUs)

	tests := []struct {
		name     string
		index    int
		wantErr  bool
		wantUUID string
	}{
		{"valid index 0", 0, false, "GPU-aaaa-1111"},
		{"valid index 1", 1, false, "GPU-bbbb-2222"},
		{"negative index", -1, true, ""},
		{"index out of range", 5, true, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := c.CollectDevice(tt.index)
			if (err != nil) != tt.wantErr {
				t.Errorf("CollectDevice(%d) error = %v, wantErr %v", tt.index, err, tt.wantErr)
				return
			}
			if !tt.wantErr && got.UUID != tt.wantUUID {
				t.Errorf("CollectDevice(%d) UUID = %v, want %v", tt.index, got.UUID, tt.wantUUID)
			}
		})
	}
}

func TestMockCollectorDeviceCount(t *testing.T) {
	tests := []struct {
		name    string
		devices []Metrics
		want    int
	}{
		{"two devices", twoGPUs, 2},
		{"no devices", []Metrics{}, 0},
		{"single device", twoGPUs[:1], 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewMockCollector(tt.devices)
			got, err := c.DeviceCount()
			if err != nil {
				t.Fatalf("DeviceCount() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("DeviceCount() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMockCollectorClose(t *testing.T) {
	c := NewMockCollector(twoGPUs)
	if err := c.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestMockCollectorImplementsInterface(t *testing.T) {
	// compile-time check that MockCollector satisfies MetricsCollector
	var _ MetricsCollector = (*MockCollector)(nil)
}

func TestMetricsFields(t *testing.T) {
	m := Metrics{
		Index:              0,
		UUID:               "GPU-test",
		Name:               "NVIDIA H100",
		GPUUtilization:     95,
		MemoryUtilization:  88,
		MemoryUsedMiB:      65536,
		MemoryTotalMiB:     81920,
		TemperatureCelsius: 80,
		PowerDrawWatts:     650,
		PowerLimitWatts:    700,
	}

	if m.GPUUtilization != 95 {
		t.Errorf("GPUUtilization = %v, want 95", m.GPUUtilization)
	}
	if m.MemoryUsedMiB != 65536 {
		t.Errorf("MemoryUsedMiB = %v, want 65536", m.MemoryUsedMiB)
	}
	if m.PowerDrawWatts > m.PowerLimitWatts {
		t.Error("PowerDrawWatts should not exceed PowerLimitWatts in normal operation")
	}
}

func TestCollectAllEmptyDevices(t *testing.T) {
	c := NewMockCollector([]Metrics{})
	got, err := c.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}
	if len(got) != 0 {
		t.Errorf("CollectAll() with no devices returned %d, want 0", len(got))
	}
}

func TestCollectDeviceBoundary(t *testing.T) {
	single := []Metrics{twoGPUs[0]}
	c := NewMockCollector(single)

	// index 0 should work
	if _, err := c.CollectDevice(0); err != nil {
		t.Errorf("CollectDevice(0) unexpected error: %v", err)
	}

	// index 1 should fail
	if _, err := c.CollectDevice(1); err == nil {
		t.Error("CollectDevice(1) should fail for single-device collector")
	}
}

// --- MIG tests ---

var migDevices = []Metrics{
	// Physical GPU 0 — MIG disabled, standard collection
	{
		Index:         0,
		UUID:          "GPU-aaaa-1111",
		Name:          "NVIDIA A100-SXM4-80GB",
		GPUUtilization: 50,
		MemoryUsedMiB: 10240,
		MemoryTotalMiB: 81920,
	},
	// MIG instance 0 on physical GPU 1
	{
		Index:              0,
		UUID:               "MIG-GPU-bbbb-2222/3/0",
		Name:               "MIG 3g.40gb",
		GPUUtilization:     85,
		MemoryUtilization:  70,
		MemoryUsedMiB:      30720,
		MemoryTotalMiB:     40960,
		TemperatureCelsius: 72, // shared from physical GPU
		PowerDrawWatts:     300,
		IsMIGInstance:      true,
		ParentIndex:        1,
		MigProfile:         "3g.40gb",
	},
	// MIG instance 1 on physical GPU 1
	{
		Index:              1,
		UUID:               "MIG-GPU-bbbb-2222/4/0",
		Name:               "MIG 3g.40gb",
		GPUUtilization:     10,
		MemoryUtilization:  5,
		MemoryUsedMiB:      2048,
		MemoryTotalMiB:     40960,
		TemperatureCelsius: 72, // shared from physical GPU
		PowerDrawWatts:     300,
		IsMIGInstance:      true,
		ParentIndex:        1,
		MigProfile:         "3g.40gb",
	},
}

func TestMockCollector_CollectAll_IncludesMIGInstances(t *testing.T) {
	c := NewMockCollector(migDevices)
	got, err := c.CollectAll()
	if err != nil {
		t.Fatalf("CollectAll() error = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("CollectAll() returned %d entries, want 3 (1 physical + 2 MIG)", len(got))
	}

	physical := got[0]
	if physical.IsMIGInstance {
		t.Error("got[0] IsMIGInstance = true, want false (physical GPU)")
	}

	mig0 := got[1]
	if !mig0.IsMIGInstance {
		t.Error("got[1] IsMIGInstance = false, want true (MIG instance)")
	}
	if mig0.MigProfile != "3g.40gb" {
		t.Errorf("got[1] MigProfile = %q, want %q", mig0.MigProfile, "3g.40gb")
	}
	if mig0.ParentIndex != 1 {
		t.Errorf("got[1] ParentIndex = %d, want 1", mig0.ParentIndex)
	}
}

func TestMockCollector_CollectByUUID_StandardGPU(t *testing.T) {
	c := NewMockCollector(migDevices)

	got, err := c.CollectByUUID("GPU-aaaa-1111")
	if err != nil {
		t.Fatalf("CollectByUUID() error = %v", err)
	}
	if got.UUID != "GPU-aaaa-1111" {
		t.Errorf("UUID = %q, want %q", got.UUID, "GPU-aaaa-1111")
	}
	if got.IsMIGInstance {
		t.Error("IsMIGInstance = true, want false for standard GPU UUID")
	}
}

func TestMockCollector_CollectByUUID_MIGInstance(t *testing.T) {
	c := NewMockCollector(migDevices)

	got, err := c.CollectByUUID("MIG-GPU-bbbb-2222/3/0")
	if err != nil {
		t.Fatalf("CollectByUUID() error = %v", err)
	}
	if !got.IsMIGInstance {
		t.Error("IsMIGInstance = false, want true for MIG UUID")
	}
	if got.MigProfile != "3g.40gb" {
		t.Errorf("MigProfile = %q, want %q", got.MigProfile, "3g.40gb")
	}
	if got.ParentIndex != 1 {
		t.Errorf("ParentIndex = %d, want 1", got.ParentIndex)
	}
	// Shared physical metrics must be copied into the MIG instance.
	if got.TemperatureCelsius != 72 {
		t.Errorf("TemperatureCelsius = %d, want 72 (shared from physical GPU)", got.TemperatureCelsius)
	}
	if got.PowerDrawWatts != 300 {
		t.Errorf("PowerDrawWatts = %d, want 300 (shared from physical GPU)", got.PowerDrawWatts)
	}
}

func TestMockCollector_CollectByUUID_NotFound(t *testing.T) {
	c := NewMockCollector(migDevices)

	_, err := c.CollectByUUID("GPU-does-not-exist")
	if err == nil {
		t.Error("CollectByUUID() expected error for unknown UUID, got nil")
	}
}

func TestMockCollector_CollectByUUID_AllMIGInstances(t *testing.T) {
	// Collect both MIG instances individually by UUID and verify they differ.
	c := NewMockCollector(migDevices)

	m0, err := c.CollectByUUID("MIG-GPU-bbbb-2222/3/0")
	if err != nil {
		t.Fatalf("CollectByUUID(inst0) error = %v", err)
	}
	m1, err := c.CollectByUUID("MIG-GPU-bbbb-2222/4/0")
	if err != nil {
		t.Fatalf("CollectByUUID(inst1) error = %v", err)
	}

	if m0.GPUUtilization == m1.GPUUtilization {
		t.Errorf("both instances have the same GPUUtilization (%d); they should differ",
			m0.GPUUtilization)
	}
	// Shared metric (temperature) must be identical across instances.
	if m0.TemperatureCelsius != m1.TemperatureCelsius {
		t.Errorf("TemperatureCelsius differs: inst0=%d inst1=%d; shared metric should be equal",
			m0.TemperatureCelsius, m1.TemperatureCelsius)
	}
}

func TestMIGMetricsFields(t *testing.T) {
	m := Metrics{
		Index:         0,
		UUID:          "MIG-GPU-cccc/1/0",
		Name:          "MIG 1g.10gb",
		IsMIGInstance: true,
		ParentIndex:   2,
		MigProfile:    "1g.10gb",
	}

	if !m.IsMIGInstance {
		t.Error("IsMIGInstance = false, want true")
	}
	if m.ParentIndex != 2 {
		t.Errorf("ParentIndex = %d, want 2", m.ParentIndex)
	}
	if m.MigProfile != "1g.10gb" {
		t.Errorf("MigProfile = %q, want %q", m.MigProfile, "1g.10gb")
	}
}

func TestParseMIGProfile(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"MIG 1g.10gb", "1g.10gb"},
		{"MIG 3g.40gb", "3g.40gb"},
		{"MIG 7g.80gb", "7g.80gb"},
		{"MIG 2g.20gb", "2g.20gb"},
		// Non-MIG name — returned unchanged
		{"NVIDIA A100-SXM4-80GB", "NVIDIA A100-SXM4-80GB"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseMIGProfile(tt.input)
			if got != tt.want {
				t.Errorf("parseMIGProfile(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMockCollectorImplementsInterfaceWithMIG(t *testing.T) {
	// compile-time check that MockCollector still satisfies MetricsCollector
	// after CollectByUUID was added to the interface.
	var _ MetricsCollector = (*MockCollector)(nil)
}
