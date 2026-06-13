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

	"github.com/pmady/keda-gpu-scaler/pkg/flux"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
	"github.com/pmady/keda-gpu-scaler/pkg/slurm"
	"go.uber.org/zap"
)

var (
	format    = flag.String("format", "table", "Output format: table, json, csv")
	interval  = flag.Duration("interval", 0, "Collection interval (0 = one-shot)")
	device    = flag.Int("device", -1, "GPU device index (-1 = all)")
	quiet     = flag.Bool("quiet", false, "Suppress log output")
	slurmMode = flag.String("slurm", "auto", "SLURM mode: auto, on, off")
	fluxMode  = flag.String("flux", "auto", "Flux mode: auto, on, off")
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
	defer func() { _ = collector.Close() }()

	// detect SLURM
	var slurmCtx *slurm.JobContext
	if useSLURM(*slurmMode) {
		ctx := slurm.FromEnv()
		slurmCtx = &ctx
		if !*quiet {
			logger.Info("SLURM job detected",
				zap.String("job_id", ctx.JobID),
				zap.String("node", ctx.NodeName),
				zap.String("gpus", ctx.GPUs))
		}
	}

	// detect Flux (mutually exclusive with SLURM in practice, but both are allowed)
	var fluxCtx *flux.JobContext
	if slurmCtx == nil && useFlux(*fluxMode) {
		ctx := flux.FromEnv()
		fluxCtx = &ctx
		if !*quiet {
			logger.Info("Flux job detected",
				zap.String("job_id", ctx.JobID),
				zap.Int("task_rank", ctx.TaskRank),
				zap.String("gpus", ctx.GPUs))
		}
	}

	// one-shot
	if *interval <= 0 {
		metrics, err := collect(collector, slurmCtx, fluxCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
			os.Exit(1)
		}
		output(metrics, *format, slurmCtx, fluxCtx)
		return
	}

	// continuous mode
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		metrics, err := collect(collector, slurmCtx, fluxCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
		} else {
			output(metrics, *format, slurmCtx, fluxCtx)
		}

		select {
		case <-sigCh:
			return
		case <-ticker.C:
		}
	}
}

func useSLURM(mode string) bool {
	switch mode {
	case "on":
		return true
	case "off":
		return false
	default: // auto
		return slurm.Detect()
	}
}

func useFlux(mode string) bool {
	switch mode {
	case "on":
		return true
	case "off":
		return false
	default: // auto
		return flux.Detect()
	}
}

// collect gathers metrics for the appropriate set of GPUs.
// Priority: --device flag > scheduler-assigned GPUs > all GPUs.
func collect(c gpu.MetricsCollector, sctx *slurm.JobContext, fctx *flux.JobContext) ([]gpu.Metrics, error) {
	if *device >= 0 {
		m, err := c.CollectDevice(*device)
		if err != nil {
			return nil, err
		}
		return []gpu.Metrics{m}, nil
	}

	// SLURM: collect only assigned GPUs
	if sctx != nil {
		if devs := sctx.VisibleDevices(); len(devs) > 0 {
			return collectDevices(c, devs)
		}
	}

	// Flux: collect only assigned GPUs
	if fctx != nil {
		if devs := fctx.VisibleDevices(); len(devs) > 0 {
			return collectDevices(c, devs)
		}
	}

	return c.CollectAll()
}

// collectDevices collects metrics for an explicit list of device indices.
func collectDevices(c gpu.MetricsCollector, devs []int) ([]gpu.Metrics, error) {
	out := make([]gpu.Metrics, 0, len(devs))
	for _, idx := range devs {
		m, err := c.CollectDevice(idx)
		if err != nil {
			return nil, fmt.Errorf("gpu %d: %w", idx, err)
		}
		out = append(out, m)
	}
	return out, nil
}

func output(metrics []gpu.Metrics, format string, sctx *slurm.JobContext, fctx *flux.JobContext) {
	switch format {
	case "json":
		outputJSON(metrics, sctx, fctx)
	case "csv":
		outputCSV(metrics, sctx, fctx)
	default:
		outputTable(metrics, sctx, fctx)
	}
}

type jsonOutput struct {
	SLURM   *slurm.JobContext `json:"slurm,omitempty"`
	Flux    *flux.JobContext  `json:"flux,omitempty"`
	Devices []gpu.Metrics     `json:"devices"`
}

func outputJSON(metrics []gpu.Metrics, sctx *slurm.JobContext, fctx *flux.JobContext) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(jsonOutput{SLURM: sctx, Flux: fctx, Devices: metrics})
}

func outputCSV(metrics []gpu.Metrics, sctx *slurm.JobContext, fctx *flux.JobContext) {
	w := csv.NewWriter(os.Stdout)
	hdr := csvHeader()
	if sctx != nil {
		hdr = append(sctx.Header(), hdr...)
	} else if fctx != nil {
		hdr = append(fctx.Header(), hdr...)
	}
	_ = w.Write(hdr)
	for _, m := range metrics {
		row := csvRow(m)
		if sctx != nil {
			row = append(sctx.Row(), row...)
		} else if fctx != nil {
			row = append(fctx.Row(), row...)
		}
		_ = w.Write(row)
	}
	w.Flush()
}

func outputTable(metrics []gpu.Metrics, sctx *slurm.JobContext, fctx *flux.JobContext) {
	if sctx != nil {
		fmt.Printf("SLURM Job %s (%s) — node %s, rank %d, gpus [%s]\n\n",
			sctx.JobID, sctx.JobName, sctx.NodeName, sctx.ProcID, sctx.GPUs)
	}
	if fctx != nil {
		fmt.Printf("Flux Job %s — task rank %d, local rank %d, gpus [%s]\n\n",
			fctx.JobID, fctx.TaskRank, fctx.LocalID, fctx.GPUs)
	}
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
