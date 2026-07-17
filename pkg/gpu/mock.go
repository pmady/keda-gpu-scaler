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
	"fmt"
	"sync"
)

// MockCollector is a test double for MetricsCollector.
type MockCollector struct {
	Devices []Metrics

	mu sync.RWMutex
	// deviceCountErr, when set via SetDeviceCountErr, is returned by
	// DeviceCount instead of the device count. Useful for simulating NVML
	// failures (e.g. in health check tests) without a real GPU.
	deviceCountErr error
}

// NewMockCollector returns a mock backed by the given devices.
func NewMockCollector(devices []Metrics) *MockCollector {
	return &MockCollector{Devices: devices}
}

// SetDeviceCountErr configures the error DeviceCount returns going forward
// (nil clears it). Safe to call concurrently with DeviceCount, so tests can
// flip NVML availability while a background health checker is polling it.
func (m *MockCollector) SetDeviceCountErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deviceCountErr = err
}

func (m *MockCollector) CollectAll() ([]Metrics, error) {
	return m.Devices, nil
}

func (m *MockCollector) CollectDevice(index int) (Metrics, error) {
	if index < 0 || index >= len(m.Devices) {
		return Metrics{}, fmt.Errorf("device index %d out of range (0-%d)", index, len(m.Devices)-1)
	}
	return m.Devices[index], nil
}

func (m *MockCollector) CollectByUUID(uuid string) (Metrics, error) {
	for _, d := range m.Devices {
		if d.UUID == uuid {
			return d, nil
		}
	}
	return Metrics{}, fmt.Errorf("device with UUID %q not found", uuid)
}

func (m *MockCollector) DeviceCount() (int, error) {
	m.mu.RLock()
	err := m.deviceCountErr
	m.mu.RUnlock()
	if err != nil {
		return 0, err
	}
	return len(m.Devices), nil
}

func (m *MockCollector) Close() error {
	return nil
}
