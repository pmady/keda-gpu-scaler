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
	"strings"

	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"go.uber.org/zap"
)

// isMIGEnabled reports whether MIG mode is currently active on device.
// Returns false when MIG is not supported by the hardware.
func isMIGEnabled(device nvml.Device) bool {
	mode, _, ret := device.GetMigMode()
	return ret == nvml.SUCCESS && mode == nvml.DEVICE_MIG_ENABLE
}

// collectMIGInstances enumerates all MIG compute instances on a physical GPU
// (parentIndex) and returns one Metrics per instance.
//
// Temperature, power, PCIe throughput, and NVLink throughput are chip-level
// resources shared across all MIG slices; they are read once from the physical
// handle and copied into every instance's Metrics so per-instance fields remain
// comparable across collection strategies.
func (c *Collector) collectMIGInstances(device nvml.Device, parentIndex int) ([]Metrics, error) {
	// Shared physical metrics — read once, copied into every instance.
	physical := c.collectPhysicalForMIG(device, parentIndex)

	var metrics []Metrics
	for idx := uint32(0); ; idx++ {
		migDevice, ret := device.GetMigDeviceHandleByIndex(idx)
		if ret != nvml.SUCCESS {
			if idx == 0 {
				// MIG is enabled but the GPU has not been partitioned yet.
				c.logger.Warn("MIG enabled but no instances found; the GPU may not be partitioned yet",
					zap.Int("gpuIndex", parentIndex))
			}
			break
		}

		m, err := c.collectMIGDevice(migDevice, parentIndex, int(idx), physical)
		if err != nil {
			c.logger.Warn("failed to collect MIG instance metrics",
				zap.Int("gpu", parentIndex),
				zap.Uint32("instanceIdx", idx),
				zap.Error(err))
			continue
		}
		metrics = append(metrics, m)
	}
	return metrics, nil
}

// collectMIGDevice reads per-instance NVML metrics from a MIG compute instance
// handle and overlays the shared physical metrics (temperature, power, bandwidth).
func (c *Collector) collectMIGDevice(device nvml.Device, parentIndex, instanceIdx int, physical Metrics) (Metrics, error) {
	m := Metrics{
		Index:         instanceIdx,
		IsMIGInstance: true,
		ParentIndex:   parentIndex,
		// Chip-level resources shared across all instances on this physical GPU.
		TemperatureCelsius: physical.TemperatureCelsius,
		PowerDrawWatts:     physical.PowerDrawWatts,
		PowerLimitWatts:    physical.PowerLimitWatts,
		PCIeTxKBps:         physical.PCIeTxKBps,
		PCIeRxKBps:         physical.PCIeRxKBps,
		NVLinkTxMBps:       physical.NVLinkTxMBps,
		NVLinkRxMBps:       physical.NVLinkRxMBps,
	}

	if uuid, ret := device.GetUUID(); ret == nvml.SUCCESS {
		m.UUID = uuid
	}

	if name, ret := device.GetName(); ret == nvml.SUCCESS {
		m.Name = name
		m.MigProfile = parseMIGProfile(name)
	}

	// Per-instance SM and memory-controller utilization.
	if util, ret := device.GetUtilizationRates(); ret == nvml.SUCCESS {
		m.GPUUtilization = util.Gpu
		m.MemoryUtilization = util.Memory
	}

	// Per-instance VRAM allocation (the MIG slice's dedicated frame buffer).
	if mem, ret := device.GetMemoryInfo(); ret == nvml.SUCCESS {
		m.MemoryUsedMiB = mem.Used / (1024 * 1024)
		m.MemoryTotalMiB = mem.Total / (1024 * 1024)
	}

	return m, nil
}

// collectPhysicalForMIG reads the chip-level metrics that are shared across
// all MIG instances on a physical GPU: temperature, power, PCIe, and NVLink.
func (c *Collector) collectPhysicalForMIG(device nvml.Device, index int) Metrics {
	m := Metrics{Index: index}

	if temp, ret := device.GetTemperature(nvml.TEMPERATURE_GPU); ret == nvml.SUCCESS {
		m.TemperatureCelsius = temp
	}
	if power, ret := device.GetPowerUsage(); ret == nvml.SUCCESS {
		m.PowerDrawWatts = power / 1000 // mW → W
	}
	if limit, ret := device.GetPowerManagementLimit(); ret == nvml.SUCCESS {
		m.PowerLimitWatts = limit / 1000
	}
	if tx, ret := device.GetPcieThroughput(nvml.PCIE_UTIL_TX_BYTES); ret == nvml.SUCCESS {
		m.PCIeTxKBps = tx
	}
	if rx, ret := device.GetPcieThroughput(nvml.PCIE_UTIL_RX_BYTES); ret == nvml.SUCCESS {
		m.PCIeRxKBps = rx
	}

	var txKBps, rxKBps uint64
	for link := 0; link < maxNVLinks; link++ {
		tx, rx, ret := nvml.DeviceGetNvLinkUtilizationCounter(device, link, 0)
		if ret != nvml.SUCCESS {
			continue
		}
		txKBps += tx
		rxKBps += rx
	}
	m.NVLinkTxMBps = txKBps / 1024
	m.NVLinkRxMBps = rxKBps / 1024

	return m
}

// parseMIGProfile extracts the profile string from an NVML MIG device name.
// NVML names MIG compute instances as "MIG 1g.10gb", "MIG 3g.40gb", etc.
// parseMIGProfile strips the "MIG " prefix and returns the profile slice.
func parseMIGProfile(name string) string {
	const prefix = "MIG "
	if strings.HasPrefix(name, prefix) {
		return strings.TrimPrefix(name, prefix)
	}
	return name
}
