//go:build mock

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

// DefaultMockDevices returns a realistic set of four NVIDIA A100-SXM4-80GB GPUs
// at varying utilisation levels, suitable for local development and testing
// without real GPU hardware.
//
// The four devices simulate a typical workload mix:
//   - GPU 0: heavy inference load (85% compute, 71% VRAM)
//   - GPU 1: moderate training load (62% compute, 48% VRAM)
//   - GPU 2: light / idle-ish (12% compute, 14% VRAM)
//   - GPU 3: completely idle (0% compute, 3% VRAM — just the driver overhead)
func DefaultMockDevices() []Metrics {
	const (
		totalMiB    = 81920 // 80 GiB — A100-SXM4-80GB
		powerLimitW = 400
	)
	return []Metrics{
		{
			Index:              0,
			UUID:               "GPU-mock-0000-0000-0000-000000000000",
			Name:               "NVIDIA A100-SXM4-80GB",
			GPUUtilization:     85,
			MemoryUtilization:  71,
			MemoryUsedMiB:      58368, // ~57 GiB
			MemoryTotalMiB:     totalMiB,
			TemperatureCelsius: 74,
			PowerDrawWatts:     342,
			PowerLimitWatts:    powerLimitW,
			PCIeTxKBps:         24576, // ~24 GB/s
			PCIeRxKBps:         18432,
			NVLinkTxMBps:       612000, // ~600 GB/s aggregate
			NVLinkRxMBps:       598000,
		},
		{
			Index:              1,
			UUID:               "GPU-mock-1111-1111-1111-111111111111",
			Name:               "NVIDIA A100-SXM4-80GB",
			GPUUtilization:     62,
			MemoryUtilization:  48,
			MemoryUsedMiB:      39321, // ~38 GiB
			MemoryTotalMiB:     totalMiB,
			TemperatureCelsius: 68,
			PowerDrawWatts:     275,
			PowerLimitWatts:    powerLimitW,
			PCIeTxKBps:         14336,
			PCIeRxKBps:         11264,
			NVLinkTxMBps:       480000,
			NVLinkRxMBps:       462000,
		},
		{
			Index:              2,
			UUID:               "GPU-mock-2222-2222-2222-222222222222",
			Name:               "NVIDIA A100-SXM4-80GB",
			GPUUtilization:     12,
			MemoryUtilization:  14,
			MemoryUsedMiB:      11469, // ~11 GiB
			MemoryTotalMiB:     totalMiB,
			TemperatureCelsius: 42,
			PowerDrawWatts:     98,
			PowerLimitWatts:    powerLimitW,
			PCIeTxKBps:         2048,
			PCIeRxKBps:         1024,
			NVLinkTxMBps:       0,
			NVLinkRxMBps:       0,
		},
		{
			Index:              3,
			UUID:               "GPU-mock-3333-3333-3333-333333333333",
			Name:               "NVIDIA A100-SXM4-80GB",
			GPUUtilization:     0,
			MemoryUtilization:  3,
			MemoryUsedMiB:      2457, // ~2.4 GiB driver overhead
			MemoryTotalMiB:     totalMiB,
			TemperatureCelsius: 31,
			PowerDrawWatts:     52,
			PowerLimitWatts:    powerLimitW,
			PCIeTxKBps:         0,
			PCIeRxKBps:         0,
			NVLinkTxMBps:       0,
			NVLinkRxMBps:       0,
		},
	}
}
