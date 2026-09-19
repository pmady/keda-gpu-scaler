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
	"os"

	"go.uber.org/zap"
)

// Vendor identifies which GPU vendor a collector talks to.
type Vendor string

const (
	VendorNVIDIA  Vendor = "nvidia"
	VendorAMD     Vendor = "amd"
	VendorUnknown Vendor = ""
)

// nvidiaDevPath and amdDevPath are the character devices used to probe for a vendor's driver.
const (
	nvidiaDevPath = "/dev/nvidiactl"
	amdDevPath    = "/dev/kfd"
)

// devExists is a func-var so tests can stub device presence without touching /dev.
var devExists = func(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DetectVendor probes for a supported GPU vendor's driver device.
func DetectVendor() Vendor {
	return detectVendor(devExists)
}

func detectVendor(exists func(string) bool) Vendor {
	if exists(nvidiaDevPath) {
		return VendorNVIDIA
	}
	if exists(amdDevPath) {
		return VendorAMD
	}
	return VendorUnknown
}

// NewCollectorForVendor builds the MetricsCollector for the given vendor.
func NewCollectorForVendor(v Vendor, logger *zap.Logger) (MetricsCollector, error) {
	switch v {
	case VendorNVIDIA:
		return NewCollector(logger)
	case VendorAMD:
		return nil, fmt.Errorf("AMD ROCm collector not implemented yet (see issue #1)")
	default:
		return nil, fmt.Errorf("unsupported GPU vendor %q", v)
	}
}

// NewDetectedCollector detects the GPU vendor on this node and builds its collector.
func NewDetectedCollector(logger *zap.Logger) (MetricsCollector, error) {
	v := DetectVendor()
	if v == VendorUnknown {
		return nil, fmt.Errorf("no supported GPU device found (checked %s, %s)", nvidiaDevPath, amdDevPath)
	}
	logger.Info("GPU vendor detected", zap.String("vendor", string(v)))
	return NewCollectorForVendor(v, logger)
}
