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

// Package triton scrapes the Prometheus metrics endpoint exposed by NVIDIA
// Triton Inference Server (default :8002/metrics) so KEDA can scale on
// Triton's own signals (queue wait time, request rate) instead of treating
// GPU utilization/memory as a proxy for load.
package triton

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// EngineMetrics holds the metrics scraped from a Triton instance. The four
// exported counters below (QueueDurationUs, RequestSuccess, RequestFailure,
// InferenceCount) are cumulative since server start, exactly as Triton
// reports them. AvgQueueWaitUs and RequestRatePerSec are derived by Client
// from two consecutive scrapes and are only meaningful once a client has
// scraped the same endpoint at least twice.
type EngineMetrics struct {
	QueueDurationUs float64 // nv_inference_queue_duration_us (cumulative, all requests)
	RequestSuccess  float64 // nv_inference_request_success (cumulative count)
	RequestFailure  float64 // nv_inference_request_failure (cumulative count)
	InferenceCount  float64 // nv_inference_count (cumulative count)
	GPUUtilization  float64 // nv_gpu_utilization (0.0-1.0), only present if Triton's GPU metrics are enabled

	// Derived from consecutive scrapes; zero on a client's first scrape of an
	// endpoint since there is no prior sample to diff against.
	AvgQueueWaitUs    float64 // (ΔQueueDurationUs / ΔInferenceCount) since the previous scrape
	RequestRatePerSec float64 // ΔInferenceCount / Δtime since the previous scrape
}

// Client scrapes the Triton Prometheus metrics endpoint and tracks the
// previous sample so it can derive rate-based metrics across calls to
// Scrape. A Client is safe for concurrent use.
type Client struct {
	endpoint   string // e.g. "http://triton-svc:8002/metrics"
	httpClient *http.Client

	mu               sync.Mutex
	havePrev         bool
	prevInferenceCnt float64
	prevQueueDurUs   float64
	prevScrapeTime   time.Time
}

// NewClient creates a Triton metrics client. endpoint is the full URL
// including path, e.g. "http://triton-svc:8002/metrics".
func NewClient(endpoint string) *Client {
	return &Client{
		endpoint: endpoint,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// Scrape fetches and parses the Triton metrics endpoint, filling in the
// derived AvgQueueWaitUs / RequestRatePerSec fields by comparing against the
// previous successful scrape of this same Client.
func (c *Client) Scrape() (EngineMetrics, error) {
	resp, err := c.httpClient.Get(c.endpoint)
	if err != nil {
		return EngineMetrics{}, fmt.Errorf("triton metrics request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return EngineMetrics{}, fmt.Errorf("triton metrics returned %d", resp.StatusCode)
	}

	m, err := parseMetrics(resp.Body)
	if err != nil {
		return EngineMetrics{}, err
	}

	c.applyDerived(&m)
	return m, nil
}

// applyDerived fills in m.AvgQueueWaitUs and m.RequestRatePerSec by diffing
// against the previous scrape, then records m as the new baseline. Both
// derived fields are left at zero when there is no previous sample, the
// inference count didn't advance, or Triton's counters appear to have reset
// (e.g. the server restarted), since a negative delta would produce a
// meaningless negative rate.
func (c *Client) applyDerived(m *EngineMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()

	if c.havePrev {
		dCount := m.InferenceCount - c.prevInferenceCnt
		dQueueUs := m.QueueDurationUs - c.prevQueueDurUs
		dt := now.Sub(c.prevScrapeTime).Seconds()

		if dCount > 0 {
			if dQueueUs > 0 {
				m.AvgQueueWaitUs = dQueueUs / dCount
			}
			if dt > 0 {
				m.RequestRatePerSec = dCount / dt
			}
		}
		// dCount <= 0 means no new inferences (or a counter reset after a
		// Triton restart); leave both derived fields at zero rather than
		// report a stale or negative rate.
	}

	c.prevInferenceCnt = m.InferenceCount
	c.prevQueueDurUs = m.QueueDurationUs
	c.prevScrapeTime = now
	c.havePrev = true
}

// parseMetrics reads Prometheus exposition text and pulls the Triton metrics
// we care about. Ignores everything else, including per-model labels: Triton
// emits one series per loaded model (e.g. nv_inference_count{model="..."}),
// so we sum across all label combinations for a given metric name to get an
// instance-wide total.
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
		case "nv_inference_queue_duration_us":
			m.QueueDurationUs += value
		case "nv_inference_request_success":
			m.RequestSuccess += value
		case "nv_inference_request_failure":
			m.RequestFailure += value
		case "nv_inference_count":
			m.InferenceCount += value
		case "nv_gpu_utilization":
			// Per-GPU gauge, not per-model; last value wins rather than
			// summing across GPUs (mirrors how the NVML collector reports a
			// single utilization value per device).
			m.GPUUtilization = value
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
