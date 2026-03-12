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
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"github.com/pmady/keda-gpu-scaler/pkg/scaler"
	pb "github.com/pmady/keda-gpu-scaler/pkg/externalscaler"
)

var (
	port     = flag.Int("port", 6000, "gRPC server port")
	logLevel = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
)

func main() {
	flag.Parse()

	logger := initLogger(*logLevel)
	defer logger.Sync()

	logger.Info("Starting keda-gpu-scaler",
		zap.Int("port", *port),
		zap.String("logLevel", *logLevel),
	)

	// Initialize NVML GPU collector
	collector, err := gpu.NewCollector(logger)
	if err != nil {
		logger.Fatal("Failed to initialize GPU collector", zap.Error(err))
	}
	defer collector.Close()

	// Log detected GPUs
	count, err := collector.DeviceCount()
	if err != nil {
		logger.Fatal("Failed to get GPU device count", zap.Error(err))
	}
	logger.Info("GPU devices detected", zap.Int("count", count))

	allMetrics, err := collector.CollectAll()
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
	gpuScaler := scaler.NewGPUExternalScaler(collector, logger)
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
