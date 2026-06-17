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

package env

import (
	"os"
	"testing"
)

// clearSchedulerEnv removes all scheduler detection variables so Detect()
// returns Standalone by default in each test.
func clearSchedulerEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"SLURM_JOB_ID",
		"FLUX_JOB_ID",
		"KUBERNETES_SERVICE_HOST",
	} {
		t.Setenv(key, "")
		os.Unsetenv(key) //nolint:errcheck
	}
}

// --- Detect() ---

func TestDetect_SLURM(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("SLURM_JOB_ID", "99")
	if got := Detect(); got != SLURM {
		t.Errorf("Detect() = %q, want %q", got, SLURM)
	}
}

func TestDetect_Flux(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("FLUX_JOB_ID", "f-abc123")
	if got := Detect(); got != Flux {
		t.Errorf("Detect() = %q, want %q", got, Flux)
	}
}

func TestDetect_Kubernetes(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	if got := Detect(); got != Kubernetes {
		t.Errorf("Detect() = %q, want %q", got, Kubernetes)
	}
}

func TestDetect_SLURMTakesPriorityOverFlux(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("SLURM_JOB_ID", "99")
	t.Setenv("FLUX_JOB_ID", "f-abc123")
	if got := Detect(); got != SLURM {
		t.Errorf("Detect() = %q, want SLURM to have priority over Flux", got)
	}
}

func TestDetect_Standalone(t *testing.T) {
	clearSchedulerEnv(t)
	if got := Detect(); got != Standalone {
		t.Errorf("Detect() = %q, want %q", got, Standalone)
	}
}

// --- Parse() ---

func TestParse(t *testing.T) {
	// Run in a clean env so "auto" → Standalone.
	clearSchedulerEnv(t)

	tests := []struct {
		input string
		want  Type
	}{
		{"k8s", Kubernetes},
		{"kubernetes", Kubernetes},
		{"slurm", SLURM},
		{"flux", Flux},
		{"standalone", Standalone},
		{"auto", Standalone},    // falls through to Detect() → Standalone in clean env
		{"unknown", Standalone}, // unrecognised → Detect() → Standalone in clean env
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := Parse(tt.input); got != tt.want {
				t.Errorf("Parse(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- FromType() ---

func TestFromType_SLURM(t *testing.T) {
	t.Setenv("SLURM_JOB_ID", "42")
	t.Setenv("SLURM_JOB_NAME", "train")
	t.Setenv("SLURM_JOB_PARTITION", "gpu")
	t.Setenv("SLURM_NODENAME", "node01")
	t.Setenv("SLURM_PROCID", "3")
	t.Setenv("SLURM_STEP_GPUS", "0,1")

	ctx := FromType(SLURM)

	if ctx.Orchestrator != "slurm" {
		t.Errorf("Orchestrator = %q, want \"slurm\"", ctx.Orchestrator)
	}
	if ctx.JobID != "42" {
		t.Errorf("JobID = %q, want \"42\"", ctx.JobID)
	}
	if ctx.Partition != "gpu" {
		t.Errorf("Partition = %q, want \"gpu\"", ctx.Partition)
	}
	if ctx.TaskRank != 3 {
		t.Errorf("TaskRank = %d, want 3", ctx.TaskRank)
	}
	if devs := ctx.VisibleDevices(); len(devs) != 2 || devs[0] != 0 || devs[1] != 1 {
		t.Errorf("VisibleDevices() = %v, want [0 1]", devs)
	}
}

func TestFromType_Flux(t *testing.T) {
	t.Setenv("FLUX_JOB_ID", "flux-xyz")
	t.Setenv("FLUX_TASK_RANK", "2")
	t.Setenv("FLUX_URI", "local:///run/flux/local")
	t.Setenv("CUDA_VISIBLE_DEVICES", "2,3")

	ctx := FromType(Flux)

	if ctx.Orchestrator != "flux" {
		t.Errorf("Orchestrator = %q, want \"flux\"", ctx.Orchestrator)
	}
	if ctx.JobID != "flux-xyz" {
		t.Errorf("JobID = %q, want \"flux-xyz\"", ctx.JobID)
	}
	if ctx.TaskRank != 2 {
		t.Errorf("TaskRank = %d, want 2", ctx.TaskRank)
	}
	if devs := ctx.VisibleDevices(); len(devs) != 2 || devs[0] != 2 || devs[1] != 3 {
		t.Errorf("VisibleDevices() = %v, want [2 3]", devs)
	}
}

func TestFromType_Kubernetes(t *testing.T) {
	t.Setenv("NODE_NAME", "gpu-node-42")
	t.Setenv("POD_NAME", "train-pod-0")
	t.Setenv("POD_NAMESPACE", "ml-workloads")

	ctx := FromType(Kubernetes)

	if ctx.Orchestrator != "k8s" {
		t.Errorf("Orchestrator = %q, want \"k8s\"", ctx.Orchestrator)
	}
	if ctx.NodeName != "gpu-node-42" {
		t.Errorf("NodeName = %q, want \"gpu-node-42\"", ctx.NodeName)
	}
	if ctx.PodName != "train-pod-0" {
		t.Errorf("PodName = %q, want \"train-pod-0\"", ctx.PodName)
	}
	if ctx.Namespace != "ml-workloads" {
		t.Errorf("Namespace = %q, want \"ml-workloads\"", ctx.Namespace)
	}
	if devs := ctx.VisibleDevices(); len(devs) != 0 {
		t.Errorf("VisibleDevices() = %v, want []", devs)
	}
}

func TestFromType_Standalone(t *testing.T) {
	ctx := FromType(Standalone)

	if ctx.Orchestrator != "standalone" {
		t.Errorf("Orchestrator = %q, want \"standalone\"", ctx.Orchestrator)
	}
	// NodeName should be non-empty (hostname).
	if ctx.NodeName == "" {
		t.Error("NodeName should be set to hostname for standalone")
	}
}

// --- Context.Header() / Row() ---

func TestContextHeaderRow_withJob(t *testing.T) {
	ctx := Context{
		Orchestrator: "slurm",
		NodeName:     "node01",
		JobID:        "99",
		TaskRank:     2,
	}

	hdr := ctx.Header()
	row := ctx.Row()

	if len(hdr) != len(row) {
		t.Fatalf("Header len %d != Row len %d", len(hdr), len(row))
	}
	if row[0] != "slurm" {
		t.Errorf("row[orchestrator] = %q, want \"slurm\"", row[0])
	}
	if row[2] != "99" {
		t.Errorf("row[job_id] = %q, want \"99\"", row[2])
	}
	if row[3] != "2" {
		t.Errorf("row[task_rank] = %q, want \"2\"", row[3])
	}
}

func TestContextHeaderRow_noJob(t *testing.T) {
	ctx := Context{
		Orchestrator: "standalone",
		NodeName:     "myhost",
	}
	row := ctx.Row()
	// task_rank column should be empty when there's no job.
	if row[3] != "" {
		t.Errorf("row[task_rank] = %q, want \"\" for standalone with no job", row[3])
	}
}

// --- VisibleDevices() ---

func TestVisibleDevices_nil(t *testing.T) {
	ctx := Context{Orchestrator: "standalone"}
	if devs := ctx.VisibleDevices(); devs != nil {
		t.Errorf("VisibleDevices() = %v, want nil", devs)
	}
}

func TestVisibleDevices_set(t *testing.T) {
	ctx := Context{visibleDevices: []int{0, 1, 2}}
	devs := ctx.VisibleDevices()
	if len(devs) != 3 {
		t.Fatalf("VisibleDevices() len = %d, want 3", len(devs))
	}
	if devs[2] != 2 {
		t.Errorf("VisibleDevices()[2] = %d, want 2", devs[2])
	}
}

// --- Detect() priority edge cases ---

func TestDetect_SLURMTakesPriorityOverKubernetes(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("SLURM_JOB_ID", "99")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	if got := Detect(); got != SLURM {
		t.Errorf("Detect() = %q, want SLURM to have priority over Kubernetes", got)
	}
}

func TestDetect_FluxTakesPriorityOverKubernetes(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("FLUX_JOB_ID", "f-abc123")
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	if got := Detect(); got != Flux {
		t.Errorf("Detect() = %q, want Flux to have priority over Kubernetes", got)
	}
}

// --- Parse() with active scheduler ---

func TestParse_autoWithSLURMActive(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("SLURM_JOB_ID", "42")
	if got := Parse("auto"); got != SLURM {
		t.Errorf("Parse(\"auto\") with SLURM active = %q, want %q", got, SLURM)
	}
}

func TestParse_autoWithFluxActive(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("FLUX_JOB_ID", "f-xyz")
	if got := Parse("auto"); got != Flux {
		t.Errorf("Parse(\"auto\") with Flux active = %q, want %q", got, Flux)
	}
}

func TestParse_autoWithKubernetesActive(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	if got := Parse("auto"); got != Kubernetes {
		t.Errorf("Parse(\"auto\") with Kubernetes active = %q, want %q", got, Kubernetes)
	}
}

// --- FromType() edge cases ---

// unsetenv removes a var for the duration of the test and restores it on cleanup.
func unsetenv(t *testing.T, key string) {
	t.Helper()
	prev, exists := os.LookupEnv(key)
	os.Unsetenv(key) //nolint:errcheck
	t.Cleanup(func() {
		if exists {
			os.Setenv(key, prev) //nolint:errcheck
		} else {
			os.Unsetenv(key) //nolint:errcheck
		}
	})
}

func TestFromType_SLURM_NoGPUs(t *testing.T) {
	// SLURM job with no GPU allocation — VisibleDevices should be nil.
	t.Setenv("SLURM_JOB_ID", "100")
	unsetenv(t, "SLURM_STEP_GPUS")
	unsetenv(t, "SLURM_JOB_GPUS")
	unsetenv(t, "GPU_DEVICE_ORDINAL")
	unsetenv(t, "CUDA_VISIBLE_DEVICES")

	ctx := FromType(SLURM)
	if devs := ctx.VisibleDevices(); devs != nil {
		t.Errorf("VisibleDevices() = %v, want nil for SLURM job with no GPU allocation", devs)
	}
}

func TestFromType_SLURM_GPUFallbackToJobGPUs(t *testing.T) {
	// SLURM_STEP_GPUS not set → should fall back to SLURM_JOB_GPUS.
	unsetenv(t, "SLURM_STEP_GPUS")
	t.Setenv("SLURM_JOB_ID", "101")
	t.Setenv("SLURM_JOB_GPUS", "2,3")
	unsetenv(t, "GPU_DEVICE_ORDINAL")
	unsetenv(t, "CUDA_VISIBLE_DEVICES")

	ctx := FromType(SLURM)
	devs := ctx.VisibleDevices()
	if len(devs) != 2 || devs[0] != 2 || devs[1] != 3 {
		t.Errorf("VisibleDevices() = %v, want [2 3] from SLURM_JOB_GPUS fallback", devs)
	}
}

func TestFromType_SLURM_GPUFallbackToCUDA(t *testing.T) {
	// No SLURM-specific GPU vars → fall back to CUDA_VISIBLE_DEVICES.
	unsetenv(t, "SLURM_STEP_GPUS")
	unsetenv(t, "SLURM_JOB_GPUS")
	unsetenv(t, "GPU_DEVICE_ORDINAL")
	t.Setenv("SLURM_JOB_ID", "102")
	t.Setenv("CUDA_VISIBLE_DEVICES", "0")

	ctx := FromType(SLURM)
	devs := ctx.VisibleDevices()
	if len(devs) != 1 || devs[0] != 0 {
		t.Errorf("VisibleDevices() = %v, want [0] from CUDA_VISIBLE_DEVICES fallback", devs)
	}
}

func TestFromType_Flux_NoGPUs(t *testing.T) {
	// Flux job submitted without -g; CUDA_VISIBLE_DEVICES is not set.
	t.Setenv("FLUX_JOB_ID", "flux-cpu-job")
	unsetenv(t, "CUDA_VISIBLE_DEVICES")

	ctx := FromType(Flux)
	if devs := ctx.VisibleDevices(); devs != nil {
		t.Errorf("VisibleDevices() = %v, want nil for Flux job with no GPU allocation", devs)
	}
}

func TestFromType_Flux_FluxURIPopulated(t *testing.T) {
	t.Setenv("FLUX_JOB_ID", "flux-uri-test")
	t.Setenv("FLUX_URI", "ssh://head.cluster:8050/var/run/flux/local")
	unsetenv(t, "CUDA_VISIBLE_DEVICES")

	ctx := FromType(Flux)
	if ctx.FluxURI != "ssh://head.cluster:8050/var/run/flux/local" {
		t.Errorf("FluxURI = %q, want ssh://head.cluster:8050/var/run/flux/local", ctx.FluxURI)
	}
}

func TestFromType_Kubernetes_NoDownwardAPI(t *testing.T) {
	// When the Downward API env vars are absent, NodeName should fall back to hostname
	// and PodName/Namespace should be empty.
	unsetenv(t, "NODE_NAME")
	unsetenv(t, "POD_NAME")
	unsetenv(t, "POD_NAMESPACE")

	ctx := FromType(Kubernetes)

	if ctx.Orchestrator != "k8s" {
		t.Errorf("Orchestrator = %q, want \"k8s\"", ctx.Orchestrator)
	}
	if ctx.NodeName == "" {
		t.Error("NodeName should fall back to hostname when NODE_NAME is not set")
	}
	if ctx.PodName != "" {
		t.Errorf("PodName = %q, want \"\" when POD_NAME not set", ctx.PodName)
	}
	if ctx.Namespace != "" {
		t.Errorf("Namespace = %q, want \"\" when POD_NAMESPACE not set", ctx.Namespace)
	}
}

// --- Context.Row() edge cases ---

func TestContextRow_taskRankZeroWithJob(t *testing.T) {
	// task_rank=0 is a valid rank (first task). Row() must emit "0", not "".
	ctx := Context{
		Orchestrator: "slurm",
		NodeName:     "node01",
		JobID:        "55",
		TaskRank:     0,
	}
	row := ctx.Row()
	if row[3] != "0" {
		t.Errorf("row[task_rank] = %q, want \"0\" when TaskRank is 0 and JobID is set", row[3])
	}
}

func TestContextRow_nodeNamePreserved(t *testing.T) {
	ctx := Context{
		Orchestrator: "k8s",
		NodeName:     "gpu-node-99",
	}
	row := ctx.Row()
	if row[1] != "gpu-node-99" {
		t.Errorf("row[node] = %q, want \"gpu-node-99\"", row[1])
	}
}

func TestContextHeaderLength(t *testing.T) {
	// Header must always have exactly 4 columns regardless of orchestrator.
	for _, orch := range []string{"slurm", "flux", "k8s", "standalone"} {
		ctx := Context{Orchestrator: orch}
		if got := len(ctx.Header()); got != 4 {
			t.Errorf("Header() for %q has %d columns, want 4", orch, got)
		}
	}
}

// --- Type constant values ---

func TestTypeConstants(t *testing.T) {
	if Kubernetes != "k8s" {
		t.Errorf("Kubernetes = %q, want \"k8s\"", Kubernetes)
	}
	if SLURM != "slurm" {
		t.Errorf("SLURM = %q, want \"slurm\"", SLURM)
	}
	if Flux != "flux" {
		t.Errorf("Flux = %q, want \"flux\"", Flux)
	}
	if Standalone != "standalone" {
		t.Errorf("Standalone = %q, want \"standalone\"", Standalone)
	}
}

// --- MIG UUID propagation ---

func TestFromType_SLURM_MIGUUIDs(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("SLURM_JOB_ID", "42")
	t.Setenv("SLURM_STEP_GPUS", "MIG-GPU-aaaa/3/0,MIG-GPU-aaaa/4/0")

	ctx := FromType(SLURM)

	got := ctx.MIGUUIDs()
	if len(got) != 2 {
		t.Fatalf("MIGUUIDs() returned %d entries, want 2: %v", len(got), got)
	}
	if got[0] != "MIG-GPU-aaaa/3/0" {
		t.Errorf("MIGUUIDs()[0] = %q, want %q", got[0], "MIG-GPU-aaaa/3/0")
	}
	if got[1] != "MIG-GPU-aaaa/4/0" {
		t.Errorf("MIGUUIDs()[1] = %q, want %q", got[1], "MIG-GPU-aaaa/4/0")
	}
}

func TestFromType_SLURM_MIGUUIDs_nilWhenIntegers(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("SLURM_JOB_ID", "43")
	t.Setenv("SLURM_STEP_GPUS", "0,1")

	ctx := FromType(SLURM)

	if got := ctx.MIGUUIDs(); got != nil {
		t.Errorf("MIGUUIDs() = %v, want nil for integer GPU list", got)
	}
}

func TestFromType_Flux_MIGUUIDs(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("FLUX_JOB_ID", "f-abc")
	t.Setenv("CUDA_VISIBLE_DEVICES", "MIG-GPU-bbbb/1/0,MIG-GPU-bbbb/2/0")

	ctx := FromType(Flux)

	got := ctx.MIGUUIDs()
	if len(got) != 2 {
		t.Fatalf("MIGUUIDs() returned %d entries, want 2: %v", len(got), got)
	}
	if got[0] != "MIG-GPU-bbbb/1/0" {
		t.Errorf("MIGUUIDs()[0] = %q, want %q", got[0], "MIG-GPU-bbbb/1/0")
	}
}

func TestFromType_Flux_MIGUUIDs_nilWhenIntegers(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("FLUX_JOB_ID", "f-def")
	t.Setenv("CUDA_VISIBLE_DEVICES", "0,1,2")

	ctx := FromType(Flux)

	if got := ctx.MIGUUIDs(); got != nil {
		t.Errorf("MIGUUIDs() = %v, want nil for integer GPU list", got)
	}
}

func TestFromType_Kubernetes_NoMIGUUIDs(t *testing.T) {
	clearSchedulerEnv(t)
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")

	ctx := FromType(Kubernetes)

	if got := ctx.MIGUUIDs(); got != nil {
		t.Errorf("MIGUUIDs() = %v, want nil for Kubernetes environment", got)
	}
}

func TestFromType_Standalone_NoMIGUUIDs(t *testing.T) {
	clearSchedulerEnv(t)

	ctx := FromType(Standalone)

	if got := ctx.MIGUUIDs(); got != nil {
		t.Errorf("MIGUUIDs() = %v, want nil for standalone environment", got)
	}
}

func TestContext_MIGUUIDs_nilByDefault(t *testing.T) {
	// Zero-value Context must return nil, not an empty slice.
	ctx := Context{}
	if got := ctx.MIGUUIDs(); got != nil {
		t.Errorf("MIGUUIDs() on zero Context = %v, want nil", got)
	}
}

func TestFromType_SLURM_MixedMIGAndInt_VisibleDevicesEmpty(t *testing.T) {
	// When all entries are MIG UUIDs, VisibleDevices should return nil and
	// MIGUUIDs should return the UUIDs.
	clearSchedulerEnv(t)
	t.Setenv("SLURM_JOB_ID", "99")
	t.Setenv("SLURM_STEP_GPUS", "MIG-GPU-aaaa/3/0,MIG-GPU-aaaa/4/0")

	ctx := FromType(SLURM)

	if got := ctx.VisibleDevices(); got != nil {
		t.Errorf("VisibleDevices() = %v, want nil when only MIG UUIDs present", got)
	}
	if got := ctx.MIGUUIDs(); len(got) != 2 {
		t.Errorf("MIGUUIDs() = %v, want 2 entries", got)
	}
}
