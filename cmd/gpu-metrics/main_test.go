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

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/pmady/keda-gpu-scaler/pkg/env"
)

func TestDescribeDeviceFilter(t *testing.T) {
	tests := []struct {
		name    string
		device  int
		visible []int
		want    string
	}{
		{"explicit device flag", 2, nil, "device 2 (from --device)"},
		{"device flag wins over scheduler assignment", 0, []int{3, 4}, "device 0 (from --device)"},
		{"scheduler-assigned devices", -1, []int{1, 2}, "scheduler-assigned devices [1 2]"},
		{"all gpus", -1, nil, "all GPUs"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeDeviceFilter(tt.device, tt.visible); got != tt.want {
				t.Errorf("describeDeviceFilter(%d, %v) = %q, want %q", tt.device, tt.visible, got, tt.want)
			}
		})
	}
}

func TestDescribeInterval(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{"zero is one-shot", 0, "one-shot (single collection)"},
		{"negative is one-shot", -5 * time.Second, "one-shot (single collection)"},
		{"positive is continuous", 2 * time.Second, "every 2s (continuous)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := describeInterval(tt.in); got != tt.want {
				t.Errorf("describeInterval(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestDescribeFormat(t *testing.T) {
	for _, f := range []string{"table", "json", "csv"} {
		if got := describeFormat(f); got != f {
			t.Errorf("describeFormat(%q) = %q, want it unchanged", f, got)
		}
	}
	if got := describeFormat("xml"); !strings.Contains(got, "unrecognized") {
		t.Errorf("describeFormat(\"xml\") = %q, want it to flag the value as unrecognized", got)
	}
}

// The issue requires the dry-run output to report the detected environment,
// output format, device filter, and interval setting.
func TestPrintDryRunReportsRequiredItems(t *testing.T) {
	var buf bytes.Buffer
	ctx := env.Context{Orchestrator: "standalone", NodeName: "dev-box"}
	printDryRun(&buf, ctx, "json", -1, 0)
	out := buf.String()

	for _, want := range []string{
		"standalone",                   // detected environment
		"dev-box",                      // node context
		"json",                         // output format
		"all GPUs",                     // device filter
		"one-shot (single collection)", // interval setting
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\n--- output ---\n%s", want, out)
		}
	}
}

// Scheduler-specific context (k8s pod/namespace, slurm partition) should appear
// in the dry-run report, mirroring the table banner.
func TestPrintDryRunRendersSchedulerContext(t *testing.T) {
	var buf bytes.Buffer
	ctx := env.Context{
		Orchestrator: "k8s",
		NodeName:     "node-1",
		JobID:        "job-7",
		TaskRank:     2,
		PodName:      "scaler-abc",
		Namespace:    "keda",
		Partition:    "gpu-debug",
	}
	printDryRun(&buf, ctx, "table", -1, 0)
	out := buf.String()

	for _, want := range []string{"node-1", "job-7", "scaler-abc", "keda", "gpu-debug"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\n--- output ---\n%s", want, out)
		}
	}
}
