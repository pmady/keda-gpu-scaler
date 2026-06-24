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

package vllm

import (
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

const sampleMetrics = `# HELP vllm:num_requests_waiting Number of requests waiting
# TYPE vllm:num_requests_waiting gauge
vllm:num_requests_waiting{model_name="meta-llama/Llama-2-7b-chat-hf"} 12
# HELP vllm:num_requests_running Number of requests running
# TYPE vllm:num_requests_running gauge
vllm:num_requests_running{model_name="meta-llama/Llama-2-7b-chat-hf"} 3
# HELP vllm:gpu_cache_usage_perc GPU cache usage
# TYPE vllm:gpu_cache_usage_perc gauge
vllm:gpu_cache_usage_perc 0.87
# HELP vllm:num_requests_swapped Number of requests swapped
# TYPE vllm:num_requests_swapped gauge
vllm:num_requests_swapped 0
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
		{"QueueDepth", m.QueueDepth, 12},
		{"RunningCount", m.RunningCount, 3},
		{"KVCacheUsage", m.KVCacheUsage, 0.87},
		{"SwappedCount", m.SwappedCount, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if math.Abs(tt.got-tt.want) > 0.001 {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}

func TestParseMetrics_NoLabels(t *testing.T) {
	input := `vllm:num_requests_waiting 5
vllm:gpu_cache_usage_perc 0.42
`
	m, err := parseMetrics(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseMetrics() error = %v", err)
	}
	if m.QueueDepth != 5 {
		t.Errorf("QueueDepth = %v, want 5", m.QueueDepth)
	}
	if math.Abs(m.KVCacheUsage-0.42) > 0.001 {
		t.Errorf("KVCacheUsage = %v, want 0.42", m.KVCacheUsage)
	}
}

func TestParseMetrics_Empty(t *testing.T) {
	m, err := parseMetrics(strings.NewReader(""))
	if err != nil {
		t.Fatalf("parseMetrics() error = %v", err)
	}
	// All zero when nothing is parsed.
	if m.QueueDepth != 0 || m.RunningCount != 0 || m.KVCacheUsage != 0 {
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
		{`vllm:num_requests_waiting{model_name="foo"} 12`, "vllm:num_requests_waiting", 12, true},
		{`vllm:gpu_cache_usage_perc 0.87`, "vllm:gpu_cache_usage_perc", 0.87, true},
		{`# TYPE vllm:num_requests_waiting gauge`, "", 0, false},
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

	c := NewClient(ts.URL, zap.NewNop())
	m, err := c.Scrape()
	if err != nil {
		t.Fatalf("Scrape() error = %v", err)
	}
	if m.QueueDepth != 12 {
		t.Errorf("QueueDepth = %v, want 12", m.QueueDepth)
	}
	if m.RunningCount != 3 {
		t.Errorf("RunningCount = %v, want 3", m.RunningCount)
	}
}

func TestClient_Scrape_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	c := NewClient(ts.URL, zap.NewNop())
	_, err := c.Scrape()
	if err == nil {
		t.Error("Scrape() expected error for 500 response, got nil")
	}
}

func TestClient_Scrape_Unreachable(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", zap.NewNop())
	_, err := c.Scrape()
	if err == nil {
		t.Error("Scrape() expected error for unreachable endpoint, got nil")
	}
}
