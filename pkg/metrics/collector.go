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

package metrics

import (
	"fmt"
	"time"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
)

// InstrumentedCollector wraps a gpu.MetricsCollector and records
// prometheus metrics on each collection call.
type InstrumentedCollector struct {
	inner gpu.MetricsCollector
}

// Wrap returns an InstrumentedCollector around the given collector.
func Wrap(c gpu.MetricsCollector) *InstrumentedCollector {
	return &InstrumentedCollector{inner: c}
}

func (ic *InstrumentedCollector) CollectAll() ([]gpu.Metrics, error) {
	start := time.Now()
	CollectionsTotal.Inc()

	all, err := ic.inner.CollectAll()
	CollectionDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		CollectionErrorsTotal.Inc()
		return nil, err
	}

	GPUDeviceCount.Set(float64(len(all)))
	for _, m := range all {
		recordGauges(m)
	}
	return all, nil
}

func (ic *InstrumentedCollector) CollectDevice(index int) (gpu.Metrics, error) {
	start := time.Now()
	CollectionsTotal.Inc()

	m, err := ic.inner.CollectDevice(index)
	CollectionDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		CollectionErrorsTotal.Inc()
		return m, err
	}

	recordGauges(m)
	return m, nil
}

func (ic *InstrumentedCollector) DeviceCount() (int, error) {
	return ic.inner.DeviceCount()
}

func (ic *InstrumentedCollector) Close() error {
	return ic.inner.Close()
}

func recordGauges(m gpu.Metrics) {
	idx := fmt.Sprintf("%d", m.Index)
	GPUUtilization.WithLabelValues(idx, m.UUID, m.Name).Set(float64(m.GPUUtilization))
	GPUMemoryUsedBytes.WithLabelValues(idx, m.UUID, m.Name).Set(float64(m.MemoryUsedMiB) * 1024 * 1024)
	GPUMemoryTotalBytes.WithLabelValues(idx, m.UUID, m.Name).Set(float64(m.MemoryTotalMiB) * 1024 * 1024)
	GPUTemperature.WithLabelValues(idx, m.UUID, m.Name).Set(float64(m.TemperatureCelsius))
	GPUPowerDraw.WithLabelValues(idx, m.UUID, m.Name).Set(float64(m.PowerDrawWatts))
}
