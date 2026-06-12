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

//go:build mock

package gpu

import "fmt"

// Compile-time assertion: MockCollector must implement MetricsCollector.
// If a new method is added to the interface and forgotten here, the build fails immediately.
var _ MetricsCollector = (*MockCollector)(nil)

// MockCollector is a test double for MetricsCollector.
type MockCollector struct {
	Devices []Metrics
}

// NewMockCollector returns a mock backed by the given devices.
func NewMockCollector(devices []Metrics) *MockCollector {
	return &MockCollector{Devices: devices}
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

func (m *MockCollector) DeviceCount() (int, error) {
	return len(m.Devices), nil
}

func (m *MockCollector) Close() error {
	return nil
}
