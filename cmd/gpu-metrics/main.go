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
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/pmady/keda-gpu-scaler/pkg/env"
	"github.com/pmady/keda-gpu-scaler/pkg/gpu"
)

var (
	format   = flag.String("format", "table", "Output format: table, json, csv")
	interval = flag.Duration("interval", 0, "Collection interval (0 = one-shot)")
	device   = flag.Int("device", -1, "GPU device index (-1 = all)")
	quiet    = flag.Bool("quiet", false, "Suppress log output")
	envFlag  = flag.String("env", "auto", "Environment: auto, k8s, slurm, flux, standalone")
	dryRun   = flag.Bool("dry-run", false, "Print the resolved config and exit without initializing NVML")
)

func main() {
	flag.Parse()

	logger := zap.NewNop()
	if !*quiet {
		l, _ := zap.NewProduction()
		logger = l
	}
	defer func() { _ = logger.Sync() }()

	// Resolve environment context once at startup.
	envType := env.Parse(*envFlag)
	envCtx := env.FromType(envType)

	// --dry-run: report the resolved collection plan and exit before touching
	// NVML, so users can verify their config on machines without a GPU/driver.
	if *dryRun {
		printDryRun(os.Stdout, envCtx, *format, *device, *interval)
		return
	}

	if !*quiet {
		logger.Info("Environment detected",
			zap.String("orchestrator", envCtx.Orchestrator),
			zap.String("node", envCtx.NodeName),
			zap.String("job_id", envCtx.JobID),
			zap.Int("task_rank", envCtx.TaskRank),
		)
	}

	collector, err := gpu.NewCollector(logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "nvml init failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = collector.Close() }()

	// one-shot
	if *interval <= 0 {
		metrics, err := collect(collector, envCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
			os.Exit(1)
		}
		output(metrics, *format, envCtx)
		return
	}

	// continuous mode
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	for {
		metrics, err := collect(collector, envCtx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "collection failed: %v\n", err)
		} else {
			output(metrics, *format, envCtx)
		}

		select {
		case <-sigCh:
			return
		case <-ticker.C:
		}
	}
}

// printDryRun reports the resolved collection plan without initializing NVML.
func printDryRun(w io.Writer, envCtx env.Context, format string, device int, interval time.Duration) {
	lines := []string{
		"Dry run: showing the resolved configuration; NVML will not be initialized.",
		"",
		fmt.Sprintf("Environment   : %s", envCtx.Orchestrator),
	}
	if envCtx.NodeName != "" {
		lines = append(lines, fmt.Sprintf("Node          : %s", envCtx.NodeName))
	}
	if envCtx.JobID != "" {
		lines = append(lines, fmt.Sprintf("Job / Rank    : %s / %d", envCtx.JobID, envCtx.TaskRank))
	}
	if envCtx.PodName != "" {
		lines = append(lines, fmt.Sprintf("Pod           : %s", envCtx.PodName))
	}
	if envCtx.Namespace != "" {
		lines = append(lines, fmt.Sprintf("Namespace     : %s", envCtx.Namespace))
	}
	if envCtx.Partition != "" {
		lines = append(lines, fmt.Sprintf("Partition     : %s", envCtx.Partition))
	}
	lines = append(lines,
		fmt.Sprintf("Output format : %s", describeFormat(format)),
		fmt.Sprintf("Device filter : %s", describeDeviceFilter(device, envCtx.VisibleDevices())),
		fmt.Sprintf("Interval      : %s", describeInterval(interval)),
	)
	_, _ = fmt.Fprintln(w, strings.Join(lines, "\n"))
}

// describeFormat returns the output format, flagging unrecognized values that
// would silently fall back to the table renderer (see output()).
func describeFormat(format string) string {
	switch format {
	case "table", "json", "csv":
		return format
	default:
		return fmt.Sprintf("%q (unrecognized — will fall back to table)", format)
	}
}

// describeDeviceFilter mirrors collect()'s selection precedence:
// --device flag > scheduler-assigned GPUs > all GPUs.
func describeDeviceFilter(device int, visible []int) string {
	if device >= 0 {
		return fmt.Sprintf("device %d (from --device)", device)
	}
	if len(visible) > 0 {
		return fmt.Sprintf("scheduler-assigned devices %v", visible)
	}
	return "all GPUs"
}

// describeInterval renders the collection cadence.
func describeInterval(d time.Duration) string {
	if d <= 0 {
		return "one-shot (single collection)"
	}
	return fmt.Sprintf("every %s (continuous)", d)
}

// collect gathers metrics for the appropriate set of GPUs.
// Priority order:
//  1. --device flag (explicit index override)
//  2. Scheduler-assigned MIG UUIDs (HPC MIG jobs: CUDA_VISIBLE_DEVICES=MIG-…)
//  3. Scheduler-assigned integer device indices (HPC integer jobs)
//  4. All GPUs on the node (CollectAll handles per-instance MIG enumeration)
func collect(c gpu.MetricsCollector, envCtx env.Context) ([]gpu.Metrics, error) {
	if *device >= 0 {
		m, err := c.CollectDevice(*device)
		if err != nil {
			return nil, err
		}
		return []gpu.Metrics{m}, nil
	}

	// HPC MIG: scheduler assigned specific MIG compute instances by UUID.
	if uuids := envCtx.MIGUUIDs(); len(uuids) > 0 {
		return collectByUUIDs(c, uuids)
	}

	// HPC integer indices.
	if devs := envCtx.VisibleDevices(); len(devs) > 0 {
		return collectDevices(c, devs)
	}

	// Default: collect all (handles MIG enumeration internally for K8s / standalone).
	return c.CollectAll()
}

// collectByUUIDs collects metrics for each MIG UUID assigned by the scheduler.
func collectByUUIDs(c gpu.MetricsCollector, uuids []string) ([]gpu.Metrics, error) {
	out := make([]gpu.Metrics, 0, len(uuids))
	for _, uuid := range uuids {
		m, err := c.CollectByUUID(uuid)
		if err != nil {
			return nil, fmt.Errorf("uuid %s: %w", uuid, err)
		}
		out = append(out, m)
	}
	return out, nil
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

func output(metrics []gpu.Metrics, format string, envCtx env.Context) {
	switch format {
	case "json":
		outputJSON(metrics, envCtx)
	case "csv":
		outputCSV(metrics, envCtx)
	default:
		outputTable(metrics, envCtx)
	}
}

// jsonOutput is the unified JSON schema emitted across all environments.
// The "environment" block lets consumers compare runs from different
// orchestrators without any schema changes.
type jsonOutput struct {
	Environment env.Context   `json:"environment"`
	CollectedAt time.Time     `json:"collected_at"`
	Devices     []gpu.Metrics `json:"devices"`
}

func outputJSON(metrics []gpu.Metrics, envCtx env.Context) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(jsonOutput{
		Environment: envCtx,
		CollectedAt: time.Now().UTC(),
		Devices:     metrics,
	})
}

func outputCSV(metrics []gpu.Metrics, envCtx env.Context) {
	w := csv.NewWriter(os.Stdout)

	// Environment columns prefix GPU columns.
	hdr := append(envCtx.Header(), csvHeader()...)
	_ = w.Write(hdr)

	for _, m := range metrics {
		row := append(envCtx.Row(), csvRow(m)...)
		_ = w.Write(row)
	}
	w.Flush()
}

func outputTable(metrics []gpu.Metrics, envCtx env.Context) {
	// Print environment banner.
	fmt.Printf("Environment : %s", envCtx.Orchestrator)
	if envCtx.NodeName != "" {
		fmt.Printf("  |  Node: %s", envCtx.NodeName)
	}
	if envCtx.JobID != "" {
		fmt.Printf("  |  Job: %s  |  Rank: %d", envCtx.JobID, envCtx.TaskRank)
	}
	if envCtx.PodName != "" {
		fmt.Printf("  |  Pod: %s", envCtx.PodName)
	}
	if envCtx.Namespace != "" {
		fmt.Printf("  |  Namespace: %s", envCtx.Namespace)
	}
	if envCtx.Partition != "" {
		fmt.Printf("  |  Partition: %s", envCtx.Partition)
	}
	fmt.Println()
	fmt.Println()

	// GPU table.
	fmt.Printf("%-5s %-22s %6s %6s %10s %10s %6s %6s %10s %10s %10s %10s\n",
		"GPU", "Name", "Util%", "Mem%", "MemUsed", "MemTotal", "Temp", "Power",
		"PCIeTx", "PCIeRx", "NVLTx", "NVLRx")
	fmt.Println("---   ----------------------  -----  -----  ---------  ---------  -----  -----  ---------  ---------  ---------  ---------")
	for _, m := range metrics {
		// Annotate MIG instances with their profile and parent GPU index.
		label := tableGPULabel(m)
		fmt.Printf("%-5s %-22s %5d%% %5d%% %7dMiB %7dMiB %4d°C %4dW %7dKB/s %7dKB/s %7dMB/s %7dMB/s\n",
			label, truncate(m.Name, 22),
			m.GPUUtilization, m.MemoryUtilization,
			m.MemoryUsedMiB, m.MemoryTotalMiB,
			m.TemperatureCelsius, m.PowerDrawWatts,
			m.PCIeTxKBps, m.PCIeRxKBps,
			m.NVLinkTxMBps, m.NVLinkRxMBps)
	}
}

// tableGPULabel returns the value printed in the GPU column.
// MIG instances are shown as "gpu<parent>/inst<idx>" (e.g. "gpu0/inst2")
// so that it is immediately clear which physical GPU the instance belongs to.
func tableGPULabel(m gpu.Metrics) string {
	if m.IsMIGInstance {
		if m.ParentIndex >= 0 {
			return fmt.Sprintf("gpu%d/inst%d", m.ParentIndex, m.Index)
		}
		return fmt.Sprintf("mig/%d", m.Index)
	}
	return strconv.Itoa(m.Index)
}

func csvHeader() []string {
	return []string{
		"index", "uuid", "name",
		"gpu_util_pct", "mem_util_pct", "mem_used_mib", "mem_total_mib",
		"temp_c", "power_w", "power_limit_w",
		"pcie_tx_kbps", "pcie_rx_kbps",
		"nvlink_tx_mbps", "nvlink_rx_mbps",
		// MIG fields (empty for non-MIG rows)
		"is_mig_instance", "parent_index", "mig_profile",
	}
}

func csvRow(m gpu.Metrics) []string {
	parentIdx := ""
	migProfile := ""
	isMIG := "false"
	if m.IsMIGInstance {
		isMIG = "true"
		parentIdx = strconv.Itoa(m.ParentIndex)
		migProfile = m.MigProfile
	}
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
		isMIG, parentIdx, migProfile,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
