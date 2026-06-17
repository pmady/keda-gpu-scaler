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

package slurm

import (
	"os"
	"strconv"
	"strings"
)

// JobContext holds SLURM job metadata from environment variables.
type JobContext struct {
	JobID     string
	JobName   string
	Partition string
	NodeList  string
	NodeName  string
	NumNodes  int
	NumTasks  int
	ProcID    int // rank within the job
	LocalID   int // rank within the node
	GPUs      string
}

// Detect returns true if running inside a SLURM job.
func Detect() bool {
	_, ok := os.LookupEnv("SLURM_JOB_ID")
	return ok
}

// FromEnv parses SLURM env vars into a JobContext.
func FromEnv() JobContext {
	return JobContext{
		JobID:     os.Getenv("SLURM_JOB_ID"),
		JobName:   os.Getenv("SLURM_JOB_NAME"),
		Partition: os.Getenv("SLURM_JOB_PARTITION"),
		NodeList:  os.Getenv("SLURM_NODELIST"),
		NodeName:  os.Getenv("SLURM_NODENAME"),
		NumNodes:  envInt("SLURM_JOB_NUM_NODES"),
		NumTasks:  envInt("SLURM_NTASKS"),
		ProcID:    envInt("SLURM_PROCID"),
		LocalID:   envInt("SLURM_LOCALID"),
		GPUs:      slurmGPUs(),
	}
}

// Header returns column names for table/CSV output.
func (j JobContext) Header() []string {
	return []string{"JobID", "JobName", "Partition", "Node", "Rank", "LocalRank", "GPUs"}
}

// Row returns the values matching Header().
func (j JobContext) Row() []string {
	return []string{
		j.JobID,
		j.JobName,
		j.Partition,
		j.NodeName,
		strconv.Itoa(j.ProcID),
		strconv.Itoa(j.LocalID),
		j.GPUs,
	}
}

func envInt(key string) int {
	v, _ := strconv.Atoi(os.Getenv(key))
	return v
}

// slurmGPUs checks common env vars for assigned GPU indices.
func slurmGPUs() string {
	if v := os.Getenv("SLURM_STEP_GPUS"); v != "" {
		return v
	}
	if v := os.Getenv("SLURM_JOB_GPUS"); v != "" {
		return v
	}
	if v := os.Getenv("GPU_DEVICE_ORDINAL"); v != "" {
		return v
	}
	if v := os.Getenv("CUDA_VISIBLE_DEVICES"); v != "" {
		return v
	}
	return ""
}

// VisibleDevices parses GPUs into integer device indices.
// MIG UUIDs (e.g. "MIG-GPU-…/3/0") are skipped; use MIGUUIDs() for those.
func (j JobContext) VisibleDevices() []int {
	if j.GPUs == "" {
		return nil
	}
	var devs []int
	for _, p := range strings.Split(j.GPUs, ",") {
		p = strings.TrimSpace(p)
		if idx, err := strconv.Atoi(p); err == nil {
			devs = append(devs, idx)
		}
	}
	return devs
}

// MIGUUIDs returns the MIG instance UUIDs from the scheduler-assigned GPU
// list (e.g. SLURM_STEP_GPUS = "MIG-GPU-aaaa/3/0,MIG-GPU-aaaa/4/0").
// Integer device entries are ignored; use VisibleDevices() for those.
func (j JobContext) MIGUUIDs() []string {
	if j.GPUs == "" {
		return nil
	}
	var uuids []string
	for _, p := range strings.Split(j.GPUs, ",") {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "MIG-") {
			uuids = append(uuids, p)
		}
	}
	return uuids
}
