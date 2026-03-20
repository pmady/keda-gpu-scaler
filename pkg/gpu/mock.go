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

import "fmt"

// MockCollector implements MetricsCollector with configurable GPU metrics.
// Used for unit tests, integration tests, and e2e tests without GPU hardware.
type MockCollector struct {
	Devices []Metrics
}

// NewMockCollector creates a mock collector with the given device metrics.
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
