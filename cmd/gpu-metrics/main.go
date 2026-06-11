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
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"go.uber.org/zap"
)

var (
	format   = flag.String("format", "table", "Output format: table, json, csv")
	interval = flag.Duration("interval", 0, "Collection interval (0 = one-shot)")
	device   = flag.Int("device", -1, "GPU device index (-1 = all)")
	quiet    = flag.Bool("quiet", false, "Suppress log output")
)

func main() {
	flag.Parse()

	logger := zap.NewNop()
	if !*quiet {
		l, _ := zap.NewProduction()
		logger = l
	}
	defer func() { _ = logger.Sync() }()

	collector, err := gpu.NewCollector(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nvml init failed: %v\n", err)
		os.Exit(1)
	}
	defer collector.Close()

	// one-shot
	if *interval <= 0 {
		metrics, err := collect(collector)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
			os.Exit(1)
		}
		output(metrics, *format)
		return
	}

	// continuous mode
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		metrics, err := collect(collector)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
		} else {
			output(metrics, *format)
		}

		select {
		case <-sigCh:
			return
		case <-ticker.C:
		}
	}
}

func collect(c gpu.MetricsCollector) ([]gpu.Metrics, error) {
	if *device >= 0 {
		m, err := c.CollectDevice(*device)
		if err != nil {
			return nil, err
		}
		return []gpu.Metrics{m}, nil
	}
	return c.CollectAll()
}

func output(metrics []gpu.Metrics, format string) {
	switch format {
	case "json":
		outputJSON(metrics)
	case "csv":
		outputCSV(metrics)
	default:
		outputTable(metrics)
	}
}

func outputJSON(metrics []gpu.Metrics) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(metrics)
}

func outputCSV(metrics []gpu.Metrics) {
	w := csv.NewWriter(os.Stdout)
	w.Write(csvHeader())
	for _, m := range metrics {
		w.Write(csvRow(m))
	}
	w.Flush()
}

func outputTable(metrics []gpu.Metrics) {
	fmt.Printf("%-5s %-20s %6s %6s %10s %10s %6s %6s %10s %10s %10s %10s\n",
		"GPU", "Name", "Util%", "Mem%", "MemUsed", "MemTotal", "Temp", "Power",
		"PCIeTx", "PCIeRx", "NVLTx", "NVLRx")
	fmt.Println("---   ----                 -----  -----  ---------  ---------  -----  -----  ---------  ---------  ---------  ---------")
	for _, m := range metrics {
		fmt.Printf("%-5d %-20s %5d%% %5d%% %7dMiB %7dMiB %4d°C %4dW %7dKB/s %7dKB/s %7dMB/s %7dMB/s\n",
			m.Index, truncate(m.Name, 20),
			m.GPUUtilization, m.MemoryUtilization,
			m.MemoryUsedMiB, m.MemoryTotalMiB,
			m.TemperatureCelsius, m.PowerDrawWatts,
			m.PCIeTxKBps, m.PCIeRxKBps,
			m.NVLinkTxMBps, m.NVLinkRxMBps)
	}
}

func csvHeader() []string {
	return []string{
		"index", "uuid", "name",
		"gpu_util_pct", "mem_util_pct", "mem_used_mib", "mem_total_mib",
		"temp_c", "power_w", "power_limit_w",
		"pcie_tx_kbps", "pcie_rx_kbps",
		"nvlink_tx_mbps", "nvlink_rx_mbps",
	}
}

func csvRow(m gpu.Metrics) []string {
	return []string{
		strconv.Itoa(m.Index), m.UUID, m.Name,
		strconv.FormatUint(uint64(m.GPUUtilization), 10),
		strconv.FormatUint(uint64(m.MemoryUtilization), 10),
		strconv.FormatUint(m.MemoryUsedMiB, 10),
		strconv.FormatUint(m.MemoryTotalMiB, 10),
		strconv.FormatUint(uint64(m.TemperatureCelsius), 10),
		strconv.FormatUint(uint64(m.PowerDrawWatts), 10),
		strconv.FormatUint(uint64(m.PowerLimitWatts), 10),
		strconv.FormatUint(uint64(m.PCIeTxKBps), 10),
		strconv.FormatUint(uint64(m.PCIeRxKBps), 10),
		strconv.FormatUint(m.NVLinkTxMBps, 10),
		strconv.FormatUint(m.NVLinkRxMBps, 10),
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
