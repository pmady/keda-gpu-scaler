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
	"syscall"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/pmady/keda-gpu-scaler/pkg/env"
	pb "github.com/pmady/keda-gpu-scaler/pkg/externalscaler"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"github.com/pmady/keda-gpu-scaler/pkg/metrics"
	"github.com/pmady/keda-gpu-scaler/pkg/probes"
	"github.com/pmady/keda-gpu-scaler/pkg/scaler"
)

var (
	port        = flag.Int("port", 6000, "gRPC server port")
	metricsPort = flag.Int("metrics-port", 9090, "Prometheus metrics HTTP port (0 to disable)")
	probePort   = flag.Int("probe-port", 8081, "Health/readiness HTTP port (0 to disable)")
	logLevel    = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	logger := initLogger(*logLevel)
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting keda-gpu-scaler",
		zap.Int("port", *port),
		zap.Int("metricsPort", *metricsPort),
		zap.Int("probePort", *probePort),
		zap.String("logLevel", *logLevel),
	)

	// Log environment metadata so operators can correlate scaler logs with
	// GPU metrics collected by the standalone binary.
	envCtx := env.FromType(env.Detect())
	logger.Info("Runtime environment",
		zap.String("orchestrator", envCtx.Orchestrator),
		zap.String("node", envCtx.NodeName),
		zap.String("pod", envCtx.PodName),
		zap.String("namespace", envCtx.Namespace),
	)

	var probeState probes.State
	if *probePort > 0 {
		probeAddr := fmt.Sprintf(":%d", *probePort)
		go func() {
			logger.Info("Probe server listening", zap.String("address", probeAddr))
			if err := http.ListenAndServe(probeAddr, probes.Handler(&probeState)); err != nil && err != http.ErrServerClosed {
				logger.Fatal("Probe server failed", zap.Error(err))
			}
		}()
	} else {
		logger.Info("Probe server disabled (probe-port=0)")
	}

	// Initialize NVML GPU collector
	collector, err := gpu.NewCollector(logger)
	if err != nil {
		logger.Fatal("Failed to initialize GPU collector", zap.Error(err))
	}
	defer func() {
		if err := collector.Close(); err != nil {
			logger.Warn("Failed to close GPU collector", zap.Error(err))
		}
	}()

	// Wrap with prometheus instrumentation if enabled
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
		probeState.MarkReady()
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
