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
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	pb "github.com/pmady/keda-gpu-scaler/pkg/externalscaler"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"github.com/pmady/keda-gpu-scaler/pkg/metrics"
	"github.com/pmady/keda-gpu-scaler/pkg/scaler"
)

var (
	port        = flag.Int("port", 6000, "gRPC server port")
	metricsPort = flag.Int("metrics-port", 9090, "Prometheus metrics HTTP port (0 to disable)")
	healthPort  = flag.Int("health-port", 8081, "HTTP health check port (/healthz liveness, /readyz readiness)")
	logLevel    = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	logger := initLogger(*logLevel)
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting keda-gpu-scaler",
		zap.Int("port", *port),
		zap.Int("metricsPort", *metricsPort),
		zap.Int("healthPort", *healthPort),
		zap.String("logLevel", *logLevel),
	)

	// nvmlReady flips to true once NVML initialises successfully.
	// /readyz blocks traffic until then; /healthz stays 200 so k8s never
	// restarts the pod just because NVML init is still in progress.
	var nvmlReady atomic.Bool
	var collector gpu.MetricsCollector

	healthMux := http.NewServeMux()
	// Liveness: is the process alive? Always 200 — no dependency checks.
	// k8s restarts the pod only if this fails (i.e. the process is hung/dead).
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	// Readiness: are dependencies ready to serve traffic?
	// Returns 503 until NVML initialises, then checks the GPU driver is reachable.
	// k8s holds traffic (no restarts) until this passes.
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if !nvmlReady.Load() {
			http.Error(w, "NVML not yet initialized", http.StatusServiceUnavailable)
			return
		}
		if _, err := collector.DeviceCount(); err != nil {
			http.Error(w, "GPU driver unreachable: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Ready"))
	})
	go func() {
		addr := fmt.Sprintf(":%d", *healthPort)
		logger.Info("Health check server listening", zap.String("address", addr))
		if err := http.ListenAndServe(addr, healthMux); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Health server failed", zap.Error(err))
		}
	}()

	// Initialize NVML GPU collector
	nvmlCollector, err := gpu.NewCollector(logger)
	if err != nil {
		logger.Fatal("Failed to initialize GPU collector", zap.Error(err))
	}
	defer func() {
		if err := nvmlCollector.Close(); err != nil {
			logger.Warn("Failed to close GPU collector", zap.Error(err))
		}
	}()
	collector = nvmlCollector
	nvmlReady.Store(true) // NVML is up — /readyz will now return 200
	logger.Info("NVML initialized, pod is ready")

	// Wrap collector with prometheus instrumentation if metrics are enabled
	var metricsCollector gpu.MetricsCollector = collector
	if *metricsPort > 0 {
		metrics.Register(prometheus.DefaultRegisterer)
		metricsCollector = metrics.Wrap(collector)

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		metricsAddr := fmt.Sprintf(":%d", *metricsPort)
		go func() {
			logger.Info("Prometheus metrics server listening", zap.String("address", metricsAddr))
			if err := http.ListenAndServe(metricsAddr, mux); err != nil && err != http.ErrServerClosed {
				logger.Fatal("Metrics server failed", zap.Error(err))
			}
		}()
	} else {
		logger.Info("Prometheus metrics disabled (metrics-port=0)")
	}

	// Log detected GPUs
	count, err := metricsCollector.DeviceCount()
	if err != nil {
		logger.Fatal("Failed to get GPU device count", zap.Error(err))
	}
	logger.Info("GPU devices detected", zap.Int("count", count))

	allMetrics, err := metricsCollector.CollectAll()
	if err != nil {
		logger.Warn("Failed to collect initial GPU metrics", zap.Error(err))
	} else {
		for _, m := range allMetrics {
			logger.Info("GPU device",
				zap.Int("index", m.Index),
				zap.String("name", m.Name),
				zap.String("uuid", m.UUID),
				zap.Uint32("gpuUtil", m.GPUUtilization),
				zap.Uint64("memUsedMiB", m.MemoryUsedMiB),
				zap.Uint64("memTotalMiB", m.MemoryTotalMiB),
			)
		}
	}

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		logger.Fatal("Failed to listen", zap.Int("port", *port), zap.Error(err))
	}

	grpcServer := grpc.NewServer()

	// Register GPU external scaler
	gpuScaler := scaler.NewGPUExternalScaler(metricsCollector, logger)
	pb.RegisterExternalScalerServer(grpcServer, gpuScaler)

	// Register health check
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	// Register reflection for debugging
	reflection.Register(grpcServer)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
		grpcServer.GracefulStop()
	}()

	logger.Info("gRPC server listening", zap.String("address", lis.Addr().String()))
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("gRPC server failed", zap.Error(err))
	}
}

func initLogger(level string) *zap.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	config := zap.Config{
		Level:       zap.NewAtomicLevelAt(zapLevel),
		Development: false,
		Encoding:    "json",
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
	}

	logger, err := config.Build()
	if err != nil {
		panic(fmt.Sprintf("failed to initialize logger: %v", err))
	}
	return logger
}
