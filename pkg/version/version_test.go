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

package version

import (
	"runtime"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	got := String("keda-gpu-scaler")

	// Must contain the binary name, version, Go version, and build date.
	for _, want := range []string{"keda-gpu-scaler", Version, runtime.Version(), BuildDate} {
		if !strings.Contains(got, want) {
			t.Errorf("String() = %q, missing %q", got, want)
		}
	}

	// Format: "<name> <version> (<goversion>, built <date>)".
	wantPrefix := "keda-gpu-scaler " + Version + " (" + runtime.Version() + ", built " + BuildDate + ")"
	if got != wantPrefix {
		t.Errorf("String() = %q, want %q", got, wantPrefix)
	}
}

func TestRequested(t *testing.T) {
	tests := []struct {
		name    string
		flagSet bool
		args    []string
		want    bool
	}{
		{"--version flag set", true, nil, true},
		{"--version flag set, args ignored", true, []string{"foo"}, true},
		{"bare version argument", false, []string{"version"}, true},
		{"bare version argument with extras", false, []string{"version", "--foo"}, true},
		{"no flag, no args", false, nil, false},
		{"no flag, empty args", false, []string{}, false},
		{"version not the first argument", false, []string{"foo", "version"}, false},
		{"unrelated argument", false, []string{"help"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Requested(tt.flagSet, tt.args); got != tt.want {
				t.Errorf("Requested(%v, %v) = %v, want %v", tt.flagSet, tt.args, got, tt.want)
			}
		})
	}
}
