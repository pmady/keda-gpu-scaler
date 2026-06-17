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
	"strings"
	"sync"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"go.uber.org/zap"
)

// maxNVLinks is the maximum number of NVLink connections per GPU (H100 upper bound).
const maxNVLinks = 18

// Metrics holds a snapshot of GPU metrics for a single device or MIG instance.
type Metrics struct {
	Index              int
	UUID               string
	Name               string
	GPUUtilization     uint32 // percentage 0-100
	MemoryUtilization  uint32 // percentage 0-100
	MemoryUsedMiB      uint64
	MemoryTotalMiB     uint64
	TemperatureCelsius uint32
	PowerDrawWatts     uint32
	PowerLimitWatts    uint32
	// PCIe throughput
	PCIeTxKBps uint32
	PCIeRxKBps uint32
	// NVLink throughput
	NVLinkTxMBps uint64
	NVLinkRxMBps uint64

	// MIG fields — zero values when IsMIGInstance is false.
	//
	// When IsMIGInstance is true, this Metrics represents one MIG compute
	// instance. Temperature, power, PCIe, and NVLink remain chip-level and
	// are shared with all other instances on the same physical GPU; the values
	// in those fields are read from the parent GPU handle.
	IsMIGInstance bool   // true when this entry is a MIG compute instance
	ParentIndex   int    // physical GPU index (meaningful only when IsMIGInstance=true)
	MigProfile    string // e.g. "1g.10gb", "3g.40gb" (set when IsMIGInstance=true)
}

// Collector wraps NVML to collect GPU metrics.
type Collector struct {
	logger *zap.Logger
	mu     sync.Mutex
}

// NewCollector creates a new GPU metrics collector.
func NewCollector(logger *zap.Logger) (*Collector, error) {
	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		return nil, fmt.Errorf("failed to initialize NVML: %v", nvml.ErrorString(ret))
	}
	logger.Info("NVML initialized successfully")
	return &Collector{logger: logger}, nil
}

// Close shuts down the NVML library.
func (c *Collector) Close() error {
	ret := nvml.Shutdown()
	if ret != nvml.SUCCESS {
		return fmt.Errorf("failed to shutdown NVML: %v", nvml.ErrorString(ret))
	}
	return nil
}

// DeviceCount returns the number of GPU devices on this node.
func (c *Collector) DeviceCount() (int, error) {
	count, ret := nvml.DeviceGetCount()
	if ret != nvml.SUCCESS {
		return 0, fmt.Errorf("failed to get device count: %v", nvml.ErrorString(ret))
	}
	return count, nil
}

// CollectAll gathers metrics from all GPU devices on this node.
//
// When MIG is enabled on a physical GPU, CollectAll enumerates the MIG compute
// instances on that GPU and returns one Metrics per instance in place of the
// physical-GPU entry. Physical GPUs with MIG disabled are collected normally.
func (c *Collector) CollectAll() ([]Metrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	count, err := c.DeviceCount()
	if err != nil {
		return nil, err
	}

	var metrics []Metrics
	for i := 0; i < count; i++ {
		device, ret := nvml.DeviceGetHandleByIndex(i)
		if ret != nvml.SUCCESS {
			c.logger.Warn("failed to get device handle",
				zap.Int("index", i),
				zap.String("nvml_error", nvml.ErrorString(ret)))
			continue
		}

		if isMIGEnabled(device) {
			// MIG path: one Metrics per compute instance.
			instanceMetrics, err := c.collectMIGInstances(device, i)
			if err != nil {
				c.logger.Warn("failed to collect MIG instances",
					zap.Int("gpu", i), zap.Error(err))
				continue
			}
			metrics = append(metrics, instanceMetrics...)
		} else {
			// Standard path: one Metrics per physical GPU.
			m, err := c.collectDevice(i)
			if err != nil {
				c.logger.Warn("failed to collect metrics for device",
					zap.Int("index", i), zap.Error(err))
				continue
			}
			metrics = append(metrics, m)
		}
	}
	return metrics, nil
}

// CollectDevice gathers metrics from a specific GPU device by index.
//
// If MIG is enabled on the target GPU, CollectDevice collects physical-level
// metrics only (temperature, power, bandwidth) and logs a warning. Use
// CollectAll or CollectByUUID for per-instance MIG metrics.
func (c *Collector) CollectDevice(index int) (Metrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Warn when MIG is active so callers know they are getting physical totals.
	if h, ret := nvml.DeviceGetHandleByIndex(index); ret == nvml.SUCCESS && isMIGEnabled(h) {
		c.logger.Warn("MIG is enabled on this GPU; CollectDevice returns physical-level metrics only — use CollectAll for per-instance MIG metrics",
			zap.Int("index", index))
	}

	return c.collectDevice(index)
}

// CollectByUUID collects metrics for a device identified by UUID.
//
// For standard GPUs, uuid is the NVML GPU UUID (e.g. "GPU-aaaa-bbbb-…").
// For MIG instances, uuid is the MIG UUID assigned by the driver
// (e.g. "MIG-GPU-aaaa-bbbb-…/3/0"). CollectByUUID is the correct entry
// point for HPC jobs where the scheduler sets CUDA_VISIBLE_DEVICES to a
// list of MIG UUIDs.
func (c *Collector) CollectByUUID(uuid string) (Metrics, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	device, ret := nvml.DeviceGetHandleByUUID(uuid)
	if ret != nvml.SUCCESS {
		return Metrics{}, fmt.Errorf("device not found for UUID %q: %v", uuid, nvml.ErrorString(ret))
	}

	if strings.HasPrefix(uuid, "MIG-") {
		// MIG instance: try to get the parent GPU for shared metrics.
		physical := Metrics{}
		if parent, pRet := nvml.DeviceGetDeviceHandleFromMigDeviceHandle(device); pRet == nvml.SUCCESS {
			physical = c.collectPhysicalForMIG(parent, -1)
		}
		// instanceIdx is not determinable from a UUID lookup alone; use 0.
		return c.collectMIGDevice(device, -1, 0, physical)
	}

	// Standard GPU: resolve index so we can use the existing collection path.
	idx, ret := device.GetIndex()
	if ret != nvml.SUCCESS {
		return Metrics{}, fmt.Errorf("cannot determine index for UUID %q: %v", uuid, nvml.ErrorString(ret))
	}
	return c.collectDevice(idx)
}

func (c *Collector) collectDevice(index int) (Metrics, error) {
	device, ret := nvml.DeviceGetHandleByIndex(index)
	if ret != nvml.SUCCESS {
		return Metrics{}, fmt.Errorf("failed to get device handle for index %d: %v", index, nvml.ErrorString(ret))
	}

	m := Metrics{Index: index}

	// UUID
	uuid, ret := device.GetUUID()
	if ret == nvml.SUCCESS {
		m.UUID = uuid
	}

	// Name
	name, ret := device.GetName()
	if ret == nvml.SUCCESS {
		m.Name = name
	}

	// Utilization rates
	utilization, ret := device.GetUtilizationRates()
	if ret == nvml.SUCCESS {
		m.GPUUtilization = utilization.Gpu
		m.MemoryUtilization = utilization.Memory
	}

	// Memory info
	memInfo, ret := device.GetMemoryInfo()
	if ret == nvml.SUCCESS {
		m.MemoryUsedMiB = memInfo.Used / (1024 * 1024)
		m.MemoryTotalMiB = memInfo.Total / (1024 * 1024)
	}

	// Temperature
	temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU)
	if ret == nvml.SUCCESS {
		m.TemperatureCelsius = temp
	}

	// Power
	power, ret := device.GetPowerUsage()
	if ret == nvml.SUCCESS {
		m.PowerDrawWatts = power / 1000 // milliwatts to watts
	}
	powerLimit, ret := device.GetPowerManagementLimit()
	if ret == nvml.SUCCESS {
		m.PowerLimitWatts = powerLimit / 1000
	}

	// PCIe throughput
	if tx, ret := device.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES); ret == nvml.SUCCESS {
		m.PCIeTxKBps = tx
	}
	if rx, ret := device.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES); ret == nvml.SUCCESS {
		m.PCIeRxKBps = rx
	}

	// NVLink throughput
	var nvlinkTxKBps, nvlinkRxKBps uint64
	activeLinks := 0
	for link := 0; link < maxNVLinks; link++ {
		tx, rx, ret := nvml.DeviceGetNvLinkUtilizationCounter(device, link, 0)
		if ret != nvml.SUCCESS {
			continue
		}
		nvlinkTxKBps += tx
		nvlinkRxKBps += rx
		activeLinks++
	}
	if activeLinks == 0 {
		c.logger.Debug("no NVLink found", zap.Int("gpuIndex", index))
	}
	m.NVLinkTxMBps = nvlinkTxKBps / 1024
	m.NVLinkRxMBps = nvlinkRxKBps / 1024

	return m, nil
}
