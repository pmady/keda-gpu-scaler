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
	"testing"

	"go.uber.org/zap"
)

func TestDetectVendor(t *testing.T) {
	tests := []struct {
		name    string
		present map[string]bool
		want    Vendor
	}{
		{"nvidiactl present", map[string]bool{nvidiaDevPath: true}, VendorNVIDIA},
		{"kfd present", map[string]bool{amdDevPath: true}, VendorAMD},
		{"both present, nvidia wins", map[string]bool{nvidiaDevPath: true, amdDevPath: true}, VendorNVIDIA},
		{"neither present", map[string]bool{}, VendorUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exists := func(path string) bool { return tt.present[path] }
			got := detectVendor(exists)
			if got != tt.want {
				t.Errorf("detectVendor() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNewCollectorForVendor(t *testing.T) {
	tests := []struct {
		name      string
		vendor    Vendor
		wantErrIn string
	}{
		{"amd not implemented", VendorAMD, "not implemented"},
		{"unknown vendor", VendorUnknown, "unsupported"},
		{"unrecognized vendor", Vendor("intel"), "unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewCollectorForVendor(tt.vendor, zap.NewNop())
			if err == nil {
				t.Fatalf("NewCollectorForVendor(%q) error = nil, want error containing %q", tt.vendor, tt.wantErrIn)
			}
			if !strings.Contains(err.Error(), tt.wantErrIn) {
				t.Errorf("NewCollectorForVendor(%q) error = %q, want it to contain %q", tt.vendor, err.Error(), tt.wantErrIn)
			}
		})
	}
}
