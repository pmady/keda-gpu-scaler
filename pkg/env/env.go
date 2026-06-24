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

// Package env detects the workload orchestrator environment and exposes a
// unified Context that callers can embed in GPU metrics output regardless of
// whether the workload runs on Kubernetes, SLURM, Flux, or bare metal.
package env

import (
	"os"
	"strconv"

	"github.com/pmady/keda-gpu-scaler/pkg/flux"
	"github.com/pmady/keda-gpu-scaler/pkg/slurm"
)

// Type identifies the workload orchestrator.
type Type string

const (
	Kubernetes Type = "k8s"
	SLURM      Type = "slurm"
	Flux       Type = "flux"
	Standalone Type = "standalone"
)

// Context holds unified environment metadata emitted alongside GPU metrics.
// All fields are orchestrator-agnostic so callers never branch on Type.
type Context struct {
	// Common
	Orchestrator string `json:"orchestrator"`
	NodeName     string `json:"node,omitempty"`
	JobID        string `json:"job_id,omitempty"`
	TaskRank     int    `json:"task_rank"` // no omitempty: rank 0 is valid (first task in a job)

	// Kubernetes-specific (set via Downward API env vars)
	PodName   string `json:"pod_name,omitempty"`
	Namespace string `json:"namespace,omitempty"`

	// SLURM-specific
	Partition string `json:"partition,omitempty"`

	// Flux-specific
	FluxURI string `json:"flux_uri,omitempty"`

	// visibleDevices is not serialised; used to restrict GPU collection to
	// scheduler-assigned integer device indices.
	visibleDevices []int

	// migUUIDs is not serialised; holds MIG instance UUIDs assigned by the
	// scheduler (e.g. CUDA_VISIBLE_DEVICES="MIG-GPU-aaaa/3/0,MIG-GPU-aaaa/4/0").
	// When non-empty it takes priority over visibleDevices for GPU collection.
	migUUIDs []string
}

// VisibleDevices returns the integer GPU device indices assigned by the scheduler.
// Returns nil when no integer assignment is detected — callers should fall back
// to MIGUUIDs() or collect all GPUs.
func (c Context) VisibleDevices() []int {
	return c.visibleDevices
}

// MIGUUIDs returns the MIG instance UUIDs assigned by the scheduler.
// Non-empty only in HPC environments (SLURM, Flux) when the job was allocated
// MIG-partitioned GPU instances. When non-empty, callers should use
// CollectByUUID instead of CollectDevice or CollectAll.
func (c Context) MIGUUIDs() []string {
	return c.migUUIDs
}

// Header returns column labels for the environment portion of CSV / table output.
func (c Context) Header() []string {
	return []string{"orchestrator", "node", "job_id", "task_rank"}
}

// Row returns values matching Header().
func (c Context) Row() []string {
	rank := ""
	if c.JobID != "" {
		rank = strconv.Itoa(c.TaskRank)
	}
	return []string{c.Orchestrator, c.NodeName, c.JobID, rank}
}

// Detect auto-detects the current environment from process env vars.
// Priority order: SLURM → Flux → Kubernetes → Standalone.
func Detect() Type {
	if slurm.Detect() {
		return SLURM
	}
	if flux.Detect() {
		return Flux
	}
	if detectK8s() {
		return Kubernetes
	}
	return Standalone
}

// detectK8s returns true when KUBERNETES_SERVICE_HOST is set, which the
// kubelet injects into every pod automatically.
func detectK8s() bool {
	_, ok := os.LookupEnv("KUBERNETES_SERVICE_HOST")
	return ok
}

// Parse converts the string value of an --env flag into a Type.
// "auto" (and any unrecognised string) triggers Detect().
func Parse(s string) Type {
	switch s {
	case "k8s", "kubernetes":
		return Kubernetes
	case "slurm":
		return SLURM
	case "flux":
		return Flux
	case "standalone":
		return Standalone
	default: // "auto" or anything unrecognised
		return Detect()
	}
}

// FromType builds a Context populated from the current process environment
// for the given orchestrator Type.
func FromType(t Type) Context {
	switch t {
	case SLURM:
		j := slurm.FromEnv()
		return Context{
			Orchestrator:   "slurm",
			NodeName:       j.NodeName,
			JobID:          j.JobID,
			TaskRank:       j.ProcID,
			Partition:      j.Partition,
			visibleDevices: j.VisibleDevices(),
			migUUIDs:       j.MIGUUIDs(),
		}

	case Flux:
		j := flux.FromEnv()
		return Context{
			Orchestrator:   "flux",
			JobID:          j.JobID,
			TaskRank:       j.TaskRank,
			FluxURI:        j.URI,
			visibleDevices: j.VisibleDevices(),
			migUUIDs:       j.MIGUUIDs(),
		}

	case Kubernetes:
		// NODE_NAME, POD_NAME, POD_NAMESPACE are injected via the Downward API.
		// Fall back to hostname if NODE_NAME is not set (e.g. minimal deployments).
		node := os.Getenv("NODE_NAME")
		if node == "" {
			node, _ = os.Hostname()
		}
		return Context{
			Orchestrator: "k8s",
			NodeName:     node,
			PodName:      os.Getenv("POD_NAME"),
			Namespace:    os.Getenv("POD_NAMESPACE"),
		}

	default: // Standalone
		node, _ := os.Hostname()
		return Context{
			Orchestrator: "standalone",
			NodeName:     node,
		}
	}
}
