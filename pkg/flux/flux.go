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

// Package flux detects and exposes Flux workload manager job context.
// It mirrors the pkg/slurm API so gpu-metrics can treat both HPC
// schedulers uniformly.
package flux

import (
	"os"
	"strconv"
	"strings"
)

// JobContext holds Flux job metadata read from environment variables.
// Flux sets these unconditionally for every task launched with `flux run` or `flux submit`.
type JobContext struct {
	JobID    string
	TaskRank int
	LocalID  int
	NumTasks int
	NumNodes int
	URI      string
	GPUs     string
}

// Detect returns true if the current process is running inside a Flux job.
func Detect() bool {
	_, ok := os.LookupEnv("FLUX_JOB_ID")
	return ok
}

// FromEnv parses the Flux environment variables into a JobContext.
func FromEnv() JobContext {
	return JobContext{
		JobID:    os.Getenv("FLUX_JOB_ID"),
		TaskRank: envInt("FLUX_TASK_RANK"),
		LocalID:  envInt("FLUX_TASK_LOCAL_ID"),
		NumTasks: envInt("FLUX_JOB_SIZE"),
		NumNodes: envInt("FLUX_JOB_NNODES"),
		URI:      os.Getenv("FLUX_URI"),
		GPUs:     fluxGPUs(),
	}
}

// Header returns column names for table/CSV output.
func (j JobContext) Header() []string {
	return []string{"FluxJobID", "TaskRank", "LocalRank", "GPUs"}
}

// Row returns the values matching Header().
func (j JobContext) Row() []string {
	return []string{
		j.JobID,
		strconv.Itoa(j.TaskRank),
		strconv.Itoa(j.LocalID),
		j.GPUs,
	}
}

// VisibleDevices parses GPUs into a slice of integer device indices.
// Non-numeric entries (e.g. MIG UUIDs) are silently skipped because
// per-instance MIG metrics are not yet supported.
func (j JobContext) VisibleDevices() []int {
	if j.GPUs == "" {
		return nil
	}
	parts := strings.Split(j.GPUs, ",")
	devs := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if idx, err := strconv.Atoi(p); err == nil {
			devs = append(devs, idx)
		}
	}
	return devs
}

// fluxGPUs reads the GPU device indices allocated to this Flux task.
// Flux sets CUDA_VISIBLE_DEVICES automatically when a job is submitted
// with -g / --gpus-per-task and GPU affinity is enabled (the default).
// There is no Flux-specific env var equivalent to SLURM_STEP_GPUS, so
// CUDA_VISIBLE_DEVICES is the canonical source.
func fluxGPUs() string {
	if v := os.Getenv("CUDA_VISIBLE_DEVICES"); v != "" {
		return v
	}
	return ""
}

func envInt(key string) int {
	v, _ := strconv.Atoi(os.Getenv(key))
	return v
}
