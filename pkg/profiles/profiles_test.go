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

package profiles

import (
	"testing"
)

func TestGetBuiltinProfiles(t *testing.T) {
	tests := []struct {
		name       string
		profile    string
		wantFound  bool
		wantMetric MetricType
		wantTarget float64
	}{
		{
			name:       "vllm-inference exists",
			profile:    "vllm-inference",
			wantFound:  true,
			wantMetric: MetricMemoryUsedPercent,
			wantTarget: 80,
		},
		{
			name:       "triton-inference exists",
			profile:    "triton-inference",
			wantFound:  true,
			wantMetric: MetricGPUUtilization,
			wantTarget: 75,
		},
		{
			name:       "training exists",
			profile:    "training",
			wantFound:  true,
			wantMetric: MetricGPUUtilization,
			wantTarget: 90,
		},
		{
			name:       "batch exists",
			profile:    "batch",
			wantFound:  true,
			wantMetric: MetricMemoryUsedPercent,
			wantTarget: 70,
		},
		{
			name:      "unknown profile not found",
			profile:   "nonexistent",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, found := Get(tt.profile)
			if found != tt.wantFound {
				t.Errorf("Get(%q) found = %v, want %v", tt.profile, found, tt.wantFound)
				return
			}
			if !found {
				return
			}
			if p.MetricType != tt.wantMetric {
				t.Errorf("Get(%q).MetricType = %v, want %v", tt.profile, p.MetricType, tt.wantMetric)
			}
			if p.TargetValue != tt.wantTarget {
				t.Errorf("Get(%q).TargetValue = %v, want %v", tt.profile, p.TargetValue, tt.wantTarget)
			}
			if p.Name != tt.profile {
				t.Errorf("Get(%q).Name = %v, want %v", tt.profile, p.Name, tt.profile)
			}
			if p.MetricName == "" {
				t.Errorf("Get(%q).MetricName should not be empty", tt.profile)
			}
		})
	}
}

func TestList(t *testing.T) {
	names := List()
	if len(names) != 4 {
		t.Errorf("List() returned %d profiles, want 4", len(names))
	}

	expected := map[string]bool{
		"vllm-inference":   false,
		"triton-inference": false,
		"training":         false,
		"batch":            false,
	}
	for _, name := range names {
		if _, ok := expected[name]; !ok {
			t.Errorf("unexpected profile name: %q", name)
		}
		expected[name] = true
	}
	for name, found := range expected {
		if !found {
			t.Errorf("missing profile name: %q", name)
		}
	}
}

func TestProfileActivationValues(t *testing.T) {
	// training profile should have 0 activation (no scale-to-zero)
	training, _ := Get("training")
	if training.ActivationValue != 0 {
		t.Errorf("training.ActivationValue = %v, want 0", training.ActivationValue)
	}

	// vllm-inference should have non-zero activation (supports scale-to-zero)
	vllm, _ := Get("vllm-inference")
	if vllm.ActivationValue <= 0 {
		t.Errorf("vllm-inference.ActivationValue = %v, want > 0", vllm.ActivationValue)
	}
}
