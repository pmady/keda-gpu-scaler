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
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// EngineMetrics holds the metrics scraped from a vLLM instance.
type EngineMetrics struct {
	QueueDepth   float64 // vllm:num_requests_waiting
	RunningCount float64 // vllm:num_requests_running
	KVCacheUsage float64 // vllm:gpu_cache_usage_perc (0.0–1.0)
	SwappedCount float64 // vllm:num_requests_swapped
}

// Client scrapes the vLLM Prometheus metrics endpoint.
type Client struct {
	endpoint   string // e.g. "http://vllm-svc:8000/metrics"
	httpClient *http.Client
	logger     *zap.Logger
}

// NewClient creates a vLLM metrics client. endpoint is the full URL
// including path, e.g. "http://vllm-svc:8000/metrics".
func NewClient(endpoint string, logger *zap.Logger) *Client {
	return &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// Scrape fetches and parses the vLLM metrics endpoint.
func (c *Client) Scrape() (EngineMetrics, error) {
	resp, err := c.httpClient.Get(c.endpoint)
	if err != nil {
		return EngineMetrics{}, fmt.Errorf("vllm metrics request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return EngineMetrics{}, fmt.Errorf("vllm metrics returned %d", resp.StatusCode)
	}

	return parseMetrics(resp.Body)
}

// parseMetrics reads Prometheus exposition text and pulls the vLLM metrics
// we care about. Ignores everything else.
func parseMetrics(r io.Reader) (EngineMetrics, error) {
	var m EngineMetrics
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 || line[0] == '#' {
			continue
		}

		name, value, ok := parseLine(line)
		if !ok {
			continue
		}

		switch name {
		case "vllm:num_requests_waiting":
			m.QueueDepth = value
		case "vllm:num_requests_running":
			m.RunningCount = value
		case "vllm:gpu_cache_usage_perc":
			m.KVCacheUsage = value
		case "vllm:num_requests_swapped":
			m.SwappedCount = value
		}
	}
	return m, scanner.Err()
}

// parseLine extracts metric name and value from a Prometheus text line.
// Handles both bare metrics ("name value") and labeled metrics ("name{...} value").
func parseLine(line string) (string, float64, bool) {
	// Strip label block if present: "name{label=val} 42" → "name 42"
	nameEnd := strings.IndexByte(line, '{')
	var rest string
	if nameEnd >= 0 {
		closing := strings.IndexByte(line[nameEnd:], '}')
		if closing < 0 {
			return "", 0, false
		}
		rest = line[:nameEnd] + line[nameEnd+closing+1:]
	} else {
		rest = line
	}

	parts := strings.Fields(rest)
	if len(parts) < 2 {
		return "", 0, false
	}
	v, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", 0, false
	}
	return parts[0], v, true
}
