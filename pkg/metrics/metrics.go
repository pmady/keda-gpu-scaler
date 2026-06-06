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
	"github.com/prometheus/client_golang/prometheus"
)

const namespace = "keda_gpu_scaler"

var (
	CollectionsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "collections_total",
		Help:      "Total number of GPU metric collections.",
	})

	CollectionErrorsTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "collection_errors_total",
		Help:      "Total number of failed GPU metric collections.",
	})

	CollectionDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "collection_duration_seconds",
		Help:      "Duration of GPU metric collection calls.",
		Buckets:   prometheus.DefBuckets,
	})

	GPUDeviceCount = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_device_count",
		Help:      "Number of GPU devices detected on this node.",
	})

	GPUUtilization = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_utilization_percent",
		Help:      "Current GPU compute utilization percentage.",
	}, []string{"gpu_index", "gpu_uuid", "gpu_name"})

	GPUMemoryUsedBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_memory_used_bytes",
		Help:      "GPU memory currently in use (bytes).",
	}, []string{"gpu_index", "gpu_uuid", "gpu_name"})

	GPUMemoryTotalBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_memory_total_bytes",
		Help:      "Total GPU memory (bytes).",
	}, []string{"gpu_index", "gpu_uuid", "gpu_name"})

	GPUTemperature = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_temperature_celsius",
		Help:      "GPU temperature in Celsius.",
	}, []string{"gpu_index", "gpu_uuid", "gpu_name"})

	GPUPowerDraw = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "gpu_power_draw_watts",
		Help:      "GPU power draw in watts.",
	}, []string{"gpu_index", "gpu_uuid", "gpu_name"})

	ScalerRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "scaler_requests_total",
		Help:      "Total gRPC requests by method.",
	}, []string{"method"})

	ScalerRequestErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "scaler_request_errors_total",
		Help:      "Total gRPC request errors by method.",
	}, []string{"method"})
)

func Register(reg prometheus.Registerer) {
	reg.MustRegister(
		CollectionsTotal,
		CollectionErrorsTotal,
		CollectionDuration,
		GPUDeviceCount,
		GPUUtilization,
		GPUMemoryUsedBytes,
		GPUMemoryTotalBytes,
		GPUTemperature,
		GPUPowerDraw,
		ScalerRequestsTotal,
		ScalerRequestErrors,
	)
}
