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

package triton

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const sampleMetrics = `# HELP nv_inference_request_success Number of successful inference requests, all batch sizes
# TYPE nv_inference_request_success counter
nv_inference_request_success{model="resnet50",version="1"} 100
# HELP nv_inference_request_failure Number of failed inference requests, all batch sizes
# TYPE nv_inference_request_failure counter
nv_inference_request_failure{model="resnet50",version="1"} 2
# HELP nv_inference_count Number of inferences performed
# TYPE nv_inference_count counter
nv_inference_count{model="resnet50",version="1"} 100
# HELP nv_inference_queue_duration_us Cumulative inference queuing duration in microseconds
# TYPE nv_inference_queue_duration_us counter
nv_inference_queue_duration_us{model="resnet50",version="1"} 5000
# HELP nv_gpu_utilization GPU utilization rate
# TYPE nv_gpu_utilization gauge
nv_gpu_utilization{gpu_uuid="GPU-abc"} 0.55
`

func TestParseMetrics(t *testing.T) {
	m, err := parseMetrics(strings.NewReader(sampleMetrics))
	if err != nil {
		t.Fatalf("parseMetrics() error = %v", err)
	}

	tests := []struct {
		name string
		got  float64
		want float64
	}{
		{"RequestSuccess", m.RequestSuccess, 100},
		{"RequestFailure", m.RequestFailure, 2},
		{"InferenceCount", m.InferenceCount, 100},
		{"QueueDurationUs", m.QueueDurationUs, 5000},
		{"GPUUtilization", m.GPUUtilization, 0.55},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if math.Abs(tt.got-tt.want) > 0.001 {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

// Triton emits one series per loaded model; parseMetrics must sum the
// per-model counters into a single instance-wide total.
func TestParseMetrics_MultipleModels(t *testing.T) {
	input := `nv_inference_count{model="resnet50",version="1"} 100
nv_inference_count{model="bert",version="2"} 40
nv_inference_queue_duration_us{model="resnet50",version="1"} 5000
nv_inference_queue_duration_us{model="bert",version="2"} 1000
nv_inference_request_success{model="resnet50",version="1"} 95
nv_inference_request_success{model="bert",version="2"} 38
`
	m, err := parseMetrics(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseMetrics() error = %v", err)
	}
	if m.InferenceCount != 140 {
		t.Errorf("InferenceCount = %v, want 140 (100+40 summed across models)", m.InferenceCount)
	}
	if m.QueueDurationUs != 6000 {
		t.Errorf("QueueDurationUs = %v, want 6000 (5000+1000 summed across models)", m.QueueDurationUs)
	}
	if m.RequestSuccess != 133 {
		t.Errorf("RequestSuccess = %v, want 133 (95+38 summed across models)", m.RequestSuccess)
	}
}

func TestParseMetrics_NoLabels(t *testing.T) {
	input := `nv_inference_count 42
nv_gpu_utilization 0.9
`
	m, err := parseMetrics(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseMetrics() error = %v", err)
	}
	if m.InferenceCount != 42 {
		t.Errorf("InferenceCount = %v, want 42", m.InferenceCount)
	}
	if math.Abs(m.GPUUtilization-0.9) > 0.001 {
		t.Errorf("GPUUtilization = %v, want 0.9", m.GPUUtilization)
	}
}

func TestParseMetrics_Empty(t *testing.T) {
	m, err := parseMetrics(strings.NewReader(""))
	if err != nil {
		t.Fatalf("parseMetrics() error = %v", err)
	}
	if m.InferenceCount != 0 || m.QueueDurationUs != 0 || m.GPUUtilization != 0 {
		t.Errorf("expected zero values for empty input, got %+v", m)
	}
}

func TestParseLine(t *testing.T) {
	tests := []struct {
		line   string
		name   string
		value  float64
		wantOK bool
	}{
		{`nv_inference_count{model="resnet50"} 100`, "nv_inference_count", 100, true},
		{`nv_gpu_utilization 0.55`, "nv_gpu_utilization", 0.55, true},
		{`# TYPE nv_inference_count counter`, "", 0, false},
		{``, "", 0, false},
		{`broken`, "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			name, value, ok := parseLine(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("parseLine(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if name != tt.name {
				t.Errorf("name = %q, want %q", name, tt.name)
			}
			if math.Abs(value-tt.value) > 0.001 {
				t.Errorf("value = %v, want %v", value, tt.value)
			}
		})
	}
}

func TestClient_Scrape(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleMetrics))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	m, err := c.Scrape()
	if err != nil {
		t.Fatalf("Scrape() error = %v", err)
	}
	if m.InferenceCount != 100 {
		t.Errorf("InferenceCount = %v, want 100", m.InferenceCount)
	}
	if m.QueueDurationUs != 5000 {
		t.Errorf("QueueDurationUs = %v, want 5000", m.QueueDurationUs)
	}
}

func TestClient_Scrape_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	_, err := c.Scrape()
	if err == nil {
		t.Error("Scrape() expected error for 500 response, got nil")
	}
}

func TestClient_Scrape_Unreachable(t *testing.T) {
	c := NewClient("http://127.0.0.1:1")
	_, err := c.Scrape()
	if err == nil {
		t.Error("Scrape() expected error for unreachable endpoint, got nil")
	}
}

// TestClient_Scrape_FirstCallHasNoDerivedMetrics verifies that a client's
// first scrape of an endpoint reports zero for the derived rate/wait metrics,
// since there is no previous sample to diff against.
func TestClient_Scrape_FirstCallHasNoDerivedMetrics(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleMetrics))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	m, err := c.Scrape()
	if err != nil {
		t.Fatalf("Scrape() error = %v", err)
	}
	if m.AvgQueueWaitUs != 0 {
		t.Errorf("AvgQueueWaitUs on first scrape = %v, want 0", m.AvgQueueWaitUs)
	}
	if m.RequestRatePerSec != 0 {
		t.Errorf("RequestRatePerSec on first scrape = %v, want 0", m.RequestRatePerSec)
	}
}

// TestClient_Scrape_DerivesRateAcrossCalls exercises the core reason
// pkg/triton exists as a stateful Client rather than a pure parser: Triton's
// nv_inference_count and nv_inference_queue_duration_us are cumulative
// counters, so a useful "requests/sec" or "average queue wait" has to be
// derived by diffing two scrapes. A fake Triton endpoint whose counters
// advance between two Scrape() calls lets us verify that math without a
// real Triton server or a live sleep in the test.
func TestClient_Scrape_DerivesRateAcrossCalls(t *testing.T) {
	var inferenceCount int64 = 100
	var queueDurationUs int64 = 5000

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		body := `nv_inference_count ` + strconv.FormatInt(atomic.LoadInt64(&inferenceCount), 10) + `
nv_inference_queue_duration_us ` + strconv.FormatInt(atomic.LoadInt64(&queueDurationUs), 10) + `
`
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)

	first, err := c.Scrape()
	if err != nil {
		t.Fatalf("first Scrape() error = %v", err)
	}
	if first.RequestRatePerSec != 0 {
		t.Fatalf("first scrape RequestRatePerSec = %v, want 0 (no baseline yet)", first.RequestRatePerSec)
	}

	// Simulate 50 more inferences taking a cumulative 2500us of queue time,
	// as if ~10ms had passed.
	atomic.AddInt64(&inferenceCount, 50)
	atomic.AddInt64(&queueDurationUs, 2500)
	time.Sleep(20 * time.Millisecond)

	second, err := c.Scrape()
	if err != nil {
		t.Fatalf("second Scrape() error = %v", err)
	}

	// avg queue wait = Δqueue_us / Δcount = 2500/50 = 50us/request
	if math.Abs(second.AvgQueueWaitUs-50) > 0.01 {
		t.Errorf("AvgQueueWaitUs = %v, want 50", second.AvgQueueWaitUs)
	}
	// rate = Δcount / Δt; with Δcount=50 over a short real sleep, rate must
	// be positive and reasonably large, not zero or negative.
	if second.RequestRatePerSec <= 0 {
		t.Errorf("RequestRatePerSec = %v, want > 0", second.RequestRatePerSec)
	}
}

// TestClient_Scrape_CounterResetDoesNotGoNegative simulates a Triton restart
// (counters reset to a smaller value) between two scrapes on the same
// Client. The derived metrics must fall back to zero rather than report a
// nonsensical negative rate.
func TestClient_Scrape_CounterResetDoesNotGoNegative(t *testing.T) {
	var inferenceCount int64 = 1000
	var queueDurationUs int64 = 50000

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		body := `nv_inference_count ` + strconv.FormatInt(atomic.LoadInt64(&inferenceCount), 10) + `
nv_inference_queue_duration_us ` + strconv.FormatInt(atomic.LoadInt64(&queueDurationUs), 10) + `
`
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()

	c := NewClient(ts.URL)
	if _, err := c.Scrape(); err != nil {
		t.Fatalf("first Scrape() error = %v", err)
	}

	// Server "restarts": counters drop back down.
	atomic.StoreInt64(&inferenceCount, 5)
	atomic.StoreInt64(&queueDurationUs, 100)

	second, err := c.Scrape()
	if err != nil {
		t.Fatalf("second Scrape() error = %v", err)
	}
	if second.AvgQueueWaitUs != 0 {
		t.Errorf("AvgQueueWaitUs after counter reset = %v, want 0", second.AvgQueueWaitUs)
	}
	if second.RequestRatePerSec != 0 {
		t.Errorf("RequestRatePerSec after counter reset = %v, want 0", second.RequestRatePerSec)
	}
}

