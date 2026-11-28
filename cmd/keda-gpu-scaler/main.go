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
	"context"
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

	"github.com/pmady/keda-gpu-scaler/pkg/agent"
	"github.com/pmady/keda-gpu-scaler/pkg/aggregator"
	"github.com/pmady/keda-gpu-scaler/pkg/env"
	pb "github.com/pmady/keda-gpu-scaler/pkg/externalscaler"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"github.com/pmady/keda-gpu-scaler/pkg/healthcheck"
	"github.com/pmady/keda-gpu-scaler/pkg/metrics"
	nodemetrics "github.com/pmady/keda-gpu-scaler/pkg/nodemetrics"
	"github.com/pmady/keda-gpu-scaler/pkg/probes"
	"github.com/pmady/keda-gpu-scaler/pkg/scaler"
	"github.com/pmady/keda-gpu-scaler/pkg/version"
)

var (
	port                = flag.Int("port", 6000, "gRPC server port")
	metricsPort         = flag.Int("metrics-port", 9090, "Prometheus metrics HTTP port (0 to disable)")
	probePort           = flag.Int("probe-port", 8081, "Health/readiness HTTP port (0 to disable)")
	logLevel            = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	healthCheckInterval = flag.Duration("health-check-interval", healthcheck.DefaultInterval, "How often to poll NVML for the gRPC health check")
	showVersion         = flag.Bool("version", false, "Print version information and exit")
	role                = flag.String("role", "combined", "Run mode: combined (local scaler, default), agent (serve this node's metrics to an aggregator), or aggregator (cluster-wide scaler)")
	agentService        = flag.String("agent-service", "", "Headless Service name of the agents, e.g. keda-gpu-scaler-agents.keda.svc.cluster.local; required with --role=aggregator")
	agentPort           = flag.Int("agent-port", 6000, "gRPC port the agents listen on")
	agentTimeout        = flag.Duration("agent-timeout", aggregator.DefaultTimeout, "How long one agent call may take before it drops out of the cycle")
	agentStaleness      = flag.Duration("agent-staleness", aggregator.DefaultStaleness, "How long the last good fleet answer is served when no agent answers (0 disables)")
	mockGPUs            = flag.Int("mock-gpus", 0, "Serve this many synthetic GPUs instead of reading NVML (0 disables; for clusters without GPU hardware)")
	mockGPUUtilization  = flag.Int("mock-gpu-utilization", 50, "GPU utilization percentage reported by synthetic GPUs when --mock-gpus is set")
)

func main() {
	flag.Parse()

	if version.Requested(*showVersion, flag.Args()) {
		fmt.Println(version.String("keda-gpu-scaler"))
		return
	}

	logger := initLogger(*logLevel)
	defer func() { _ = logger.Sync() }()

	logger.Info("Starting keda-gpu-scaler",
		zap.String("role", *role),
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

	// Initialize the collector. In aggregator mode it fans out to the agents
	// instead of reading local device files; in the other two modes it reads
	// this node, either from NVML or from synthetic devices.
	var metricsCollector gpu.MetricsCollector
	switch *role {
	case "combined", "agent":
		metricsCollector = newLocalCollector(logger)
	case "aggregator":
		if *agentService == "" {
			logger.Fatal("--agent-service is required when --role=aggregator")
		}
		logger.Info("Aggregating GPU metrics from agents",
			zap.String("service", *agentService),
			zap.Int("agentPort", *agentPort),
			zap.Duration("timeout", *agentTimeout),
			zap.Duration("staleness", *agentStaleness),
		)
		metricsCollector = aggregator.New(aggregator.Config{
			Resolve:   aggregator.NewDNSResolver(*agentService, *agentPort),
			Timeout:   *agentTimeout,
			Staleness: *agentStaleness,
			Logger:    logger,
		})
	default:
		logger.Fatal("Unknown --role, want combined, agent or aggregator", zap.String("role", *role))
	}

	defer func() {
		if closeErr := metricsCollector.Close(); closeErr != nil {
			logger.Warn("Failed to close GPU collector", zap.Error(closeErr))
		}
	}()

	// Wrap with prometheus instrumentation if enabled
	if *metricsPort > 0 {
		metrics.Register(prometheus.DefaultRegisterer)
		metricsCollector = metrics.Wrap(metricsCollector)

		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.Handler())
		metricsAddr := fmt.Sprintf(":%d", *metricsPort)
		go func() {
			logger.Info("Prometheus metrics server listening", zap.String("address", metricsAddr))
			if listenErr := http.ListenAndServe(metricsAddr, mux); listenErr != nil && listenErr != http.ErrServerClosed {
				logger.Fatal("Metrics server failed", zap.Error(listenErr))
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

	// The aggregator cannot know at startup whether its agents are up yet, and
	// readiness that never recovers would keep KEDA off the Service for good.
	// Agent availability is reported per poll and through the gRPC health check.
	if *role == "aggregator" {
		probeState.MarkReady()
	}

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		logger.Fatal("Failed to listen", zap.Int("port", *port), zap.Error(err))
	}

	grpcServer := grpc.NewServer()

	// An agent serves the internal per-node contract; the other two modes serve
	// the KEDA ExternalScaler contract.
	if *role == "agent" {
		nodemetrics.RegisterNodeMetricsServer(grpcServer, agent.NewServer(metricsCollector, envCtx.NodeName, logger))
	} else {
		gpuScaler := scaler.NewGPUExternalScaler(metricsCollector, logger)
		pb.RegisterExternalScalerServer(grpcServer, gpuScaler)
	}

	// Register health check. The gRPC Health Checking Protocol status for the
	// server-wide service ("") tracks NVML availability: a background checker
	// polls metricsCollector.DeviceCount() on healthCheckInterval and reports
	// SERVING/NOT_SERVING accordingly (see pkg/healthcheck).
	healthServer := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	checkerCtx, cancelChecker := context.WithCancel(context.Background())
	defer cancelChecker()
	checker := healthcheck.New(metricsCollector, healthServer, *healthCheckInterval, logger)
	go checker.Run(checkerCtx)

	// Register reflection for debugging
	reflection.Register(grpcServer)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Info("Received shutdown signal", zap.String("signal", sig.String()))
		cancelChecker()
		grpcServer.GracefulStop()
	}()

	logger.Info("gRPC server listening", zap.String("address", lis.Addr().String()))
	if err := grpcServer.Serve(lis); err != nil {
		logger.Fatal("gRPC server failed", zap.Error(err))
	}
}

// newLocalCollector reads this node's GPUs: synthetic devices when --mock-gpus
// is set, otherwise the vendor auto-detected from the host's device files.
func newLocalCollector(logger *zap.Logger) gpu.MetricsCollector {
	if *mockGPUs > 0 {
		if *mockGPUUtilization < 0 || *mockGPUUtilization > 100 {
			logger.Fatal("--mock-gpu-utilization must be between 0 and 100", zap.Int("value", *mockGPUUtilization))
		}
		logger.Warn("Serving synthetic GPU metrics; no hardware is being read",
			zap.Int("mockGPUs", *mockGPUs),
			zap.Int("mockGPUUtilization", *mockGPUUtilization),
		)
		return gpu.NewMockCollector(gpu.NewMockDevices(*mockGPUs, uint32(*mockGPUUtilization)))
	}

	detected, err := gpu.NewDetectedCollector(logger)
	if err != nil {
		logger.Fatal("Failed to initialize GPU collector", zap.Error(err))
	}
	return detected
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
