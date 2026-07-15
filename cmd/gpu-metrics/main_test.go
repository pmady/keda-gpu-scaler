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
	"encoding/json"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/pmady/keda-gpu-scaler/pkg/env"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
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

// The driver version (issue #68) must appear in the JSON output when present
// and be omitted when empty (omitempty).
func TestJSONOutputDriverVersion(t *testing.T) {
	withDriver, err := json.Marshal(jsonOutput{DriverVersion: "535.104.05"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(withDriver), `"driver_version":"535.104.05"`) {
		t.Errorf("driver_version missing from JSON output: %s", withDriver)
	}

	noDriver, err := json.Marshal(jsonOutput{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(noDriver), "driver_version") {
		t.Errorf("empty driver_version should be omitted, got: %s", noDriver)
	}
}

// captureStdout redirects os.Stdout while fn runs and returns what it wrote.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	_ = w.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("copy: %v", err)
	}
	return buf.String()
}

// The driver version must appear in the table banner when present and be
// omitted when empty (issue #68).
func TestOutputTableDriverVersion(t *testing.T) {
	devices := []gpu.Metrics{{Index: 0, Name: "A100"}}

	withDriver := captureStdout(t, func() {
		outputTable(devices, env.Context{Orchestrator: "standalone"}, "535.104.05")
	})
	if !strings.Contains(withDriver, "Driver: 535.104.05") {
		t.Errorf("table banner missing driver version\n--- output ---\n%s", withDriver)
	}

	noDriver := captureStdout(t, func() {
		outputTable(devices, env.Context{Orchestrator: "standalone"}, "")
	})
	if strings.Contains(noDriver, "Driver:") {
		t.Errorf("empty driver version should not appear in banner\n--- output ---\n%s", noDriver)
	}
}

// The driver version must be a CSV column (header + value) for parity with the
// JSON and table outputs.
func TestOutputCSVDriverVersion(t *testing.T) {
	out := captureStdout(t, func() {
		outputCSV([]gpu.Metrics{{Index: 0, Name: "A100"}}, env.Context{Orchestrator: "standalone"}, "535.104.05")
	})
	if !strings.Contains(out, "driver_version") {
		t.Errorf("CSV header missing driver_version column\n--- output ---\n%s", out)
	}
	if !strings.Contains(out, "535.104.05") {
		t.Errorf("CSV row missing driver version value\n--- output ---\n%s", out)
	}
}

// countingCollector wraps a MockCollector and counts CollectAll calls, so
// tests can assert that no further collection happens after shutdown.
type countingCollector struct {
	*gpu.MockCollector
	calls int32
}

func (c *countingCollector) CollectAll() ([]gpu.Metrics, error) {
	atomic.AddInt32(&c.calls, 1)
	return c.MockCollector.CollectAll()
}

// runContinuous is exercised as gpu-metrics's Flux coprocess code path (see
// docs/hpc.md and deploy/flux/gpu-monitor.lua): Flux sends SIGTERM to the
// coprocess as soon as the job's tasks complete, and escalates to SIGKILL
// after a timeout if it hasn't exited by then. These tests validate that
// gpu-metrics does not need to be SIGKILLed — it exits promptly and cleanly
// on SIGTERM (issue #60).
func TestRunContinuousExitsCleanlyOnSIGTERM(t *testing.T) {
	collector := gpu.NewMockCollector([]gpu.Metrics{{Index: 0, Name: "A100"}})
	sigCh := make(chan os.Signal, 1)
	envCtx := env.Context{Orchestrator: "flux", JobID: "f23r45t"}

	done := make(chan struct{})
	var out string
	go func() {
		out = captureStdout(t, func() {
			runContinuous(collector, envCtx, "json", "", 5*time.Millisecond, sigCh)
		})
		close(done)
	}()

	// Let it collect at least once, then simulate Flux delivering SIGTERM to
	// the coprocess when the job's tasks complete.
	time.Sleep(20 * time.Millisecond)
	sigCh <- syscall.SIGTERM

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runContinuous did not return within 1s of SIGTERM; shutdown is not clean")
	}

	// Output is pretty-printed (enc.SetIndent("", "  ")), so keys are
	// rendered as `"key": "value"` with a space after the colon.
	if !strings.Contains(out, `"orchestrator": "flux"`) {
		t.Errorf("expected at least one JSON sample before shutdown, got: %s", out)
	}
}

func TestRunContinuousStopsCollectingAfterSIGTERM(t *testing.T) {
	collector := &countingCollector{MockCollector: gpu.NewMockCollector([]gpu.Metrics{{Index: 0}})}
	sigCh := make(chan os.Signal, 1)
	envCtx := env.Context{Orchestrator: "flux", JobID: "f23r45t"}

	done := make(chan struct{})
	go func() {
		captureStdout(t, func() {
			runContinuous(collector, envCtx, "json", "", 5*time.Millisecond, sigCh)
		})
		close(done)
	}()

	time.Sleep(20 * time.Millisecond)
	sigCh <- syscall.SIGTERM

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runContinuous did not return within 1s of SIGTERM")
	}

	callsAtShutdown := atomic.LoadInt32(&collector.calls)
	// Long enough for a leaked ticker/goroutine to fire again if the ticker
	// wasn't stopped and the loop didn't actually return.
	time.Sleep(30 * time.Millisecond)
	if got := atomic.LoadInt32(&collector.calls); got != callsAtShutdown {
		t.Errorf("collector was polled %d more time(s) after SIGTERM; ticker/goroutine not stopped cleanly", got-callsAtShutdown)
	}
}
