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

package healthcheck

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
)

func servingStatus(t *testing.T, hs *health.Server) healthpb.HealthCheckResponse_ServingStatus {
	t.Helper()
	resp, err := hs.Check(context.Background(), &healthpb.HealthCheckRequest{Service: ServiceName})
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}
	return resp.Status
}

func TestCheck_ServingWhenNVMLHealthy(t *testing.T) {
	mock := gpu.NewMockCollector([]gpu.Metrics{{Index: 0}})
	hs := health.NewServer()
	c := New(mock, hs, time.Second, zaptest.NewLogger(t))

	if got := c.Check(); got != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("Check() = %v, want SERVING", got)
	}
	if got := servingStatus(t, hs); got != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health server status = %v, want SERVING", got)
	}
}

func TestCheck_NotServingWhenNVMLFails(t *testing.T) {
	mock := gpu.NewMockCollector([]gpu.Metrics{{Index: 0}})
	mock.SetDeviceCountErr(errors.New("nvml: device count failed"))
	hs := health.NewServer()
	c := New(mock, hs, time.Second, zaptest.NewLogger(t))

	if got := c.Check(); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("Check() = %v, want NOT_SERVING", got)
	}
	if got := servingStatus(t, hs); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("health server status = %v, want NOT_SERVING", got)
	}
}

// TestCheck_Transitions verifies the health status flips both ways as NVML
// availability changes over successive checks, matching the acceptance
// criteria in issue #109 ("Health status reflects NVML availability").
func TestCheck_Transitions(t *testing.T) {
	mock := gpu.NewMockCollector([]gpu.Metrics{{Index: 0}})
	hs := health.NewServer()
	c := New(mock, hs, time.Second, zaptest.NewLogger(t))

	// Starts healthy.
	if got := c.Check(); got != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("initial Check() = %v, want SERVING", got)
	}

	// NVML starts failing (e.g. driver reset, GPU falls off the bus).
	mock.SetDeviceCountErr(errors.New("nvml: unknown error"))
	if got := c.Check(); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("Check() after NVML failure = %v, want NOT_SERVING", got)
	}
	if got := servingStatus(t, hs); got != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("health server status after failure = %v, want NOT_SERVING", got)
	}

	// NVML recovers.
	mock.SetDeviceCountErr(nil)
	if got := c.Check(); got != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("Check() after recovery = %v, want SERVING", got)
	}
	if got := servingStatus(t, hs); got != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("health server status after recovery = %v, want SERVING", got)
	}
}

func TestRun_PeriodicallyChecksAndRespectsContextCancellation(t *testing.T) {
	mock := gpu.NewMockCollector([]gpu.Metrics{{Index: 0}})
	hs := health.NewServer()
	c := New(mock, hs, 10*time.Millisecond, zaptest.NewLogger(t))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	// The immediate check on entry to Run should report SERVING right away.
	deadline := time.After(time.Second)
	for {
		if servingStatus(t, hs) == healthpb.HealthCheckResponse_SERVING {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for initial SERVING status")
		case <-time.After(time.Millisecond):
		}
	}

	// Introduce a failure and confirm the periodic loop picks it up.
	mock.SetDeviceCountErr(errors.New("nvml: boom"))
	deadline = time.After(time.Second)
	for {
		if servingStatus(t, hs) == healthpb.HealthCheckResponse_NOT_SERVING {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for NOT_SERVING status")
		case <-time.After(time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not return after context cancellation")
	}
}

func TestNew_DefaultsInterval(t *testing.T) {
	mock := gpu.NewMockCollector(nil)
	hs := health.NewServer()

	c := New(mock, hs, 0, nil)
	if c.interval != DefaultInterval {
		t.Fatalf("interval = %v, want default %v", c.interval, DefaultInterval)
	}
	if c.logger == nil {
		t.Fatal("logger should default to a no-op logger, not nil")
	}
}
