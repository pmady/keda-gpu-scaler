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
	"testing"

	"github.com/stretchr/testify/assert"
)

func setEnv(t *testing.T, kvs map[string]string) {
	t.Helper()
	for k, v := range kvs {
		t.Setenv(k, v)
	}
}

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{
			name: "inside slurm job",
			env:  map[string]string{"SLURM_JOB_ID": "12345"},
			want: true,
		},
		{
			name: "outside slurm",
			env:  map[string]string{},
			want: false,
		},
		{
			name: "empty job id still counts",
			env:  map[string]string{"SLURM_JOB_ID": ""},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			setEnv(t, tt.env)
			assert.Equal(t, tt.want, Detect())
		})
	}
}

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want JobContext
	}{
		{
			name: "full slurm environment",
			env: map[string]string{
				"SLURM_JOB_ID":        "98765",
				"SLURM_JOB_NAME":      "train-llm",
				"SLURM_JOB_PARTITION": "gpu-a100",
				"SLURM_NODELIST":      "node[01-04]",
				"SLURM_NODENAME":      "node02",
				"SLURM_JOB_NUM_NODES": "4",
				"SLURM_NTASKS":        "32",
				"SLURM_PROCID":        "8",
				"SLURM_LOCALID":       "2",
				"SLURM_STEP_GPUS":     "0,1,2,3",
			},
			want: JobContext{
				JobID:     "98765",
				JobName:   "train-llm",
				Partition: "gpu-a100",
				NodeList:  "node[01-04]",
				NodeName:  "node02",
				NumNodes:  4,
				NumTasks:  32,
				ProcID:    8,
				LocalID:   2,
				GPUs:      "0,1,2,3",
			},
		},
		{
			name: "minimal - job id only",
			env: map[string]string{
				"SLURM_JOB_ID": "111",
			},
			want: JobContext{
				JobID: "111",
			},
		},
		{
			name: "empty env",
			env:  map[string]string{},
			want: JobContext{},
		},
		{
			name: "gpu fallback to CUDA_VISIBLE_DEVICES",
			env: map[string]string{
				"SLURM_JOB_ID":         "222",
				"CUDA_VISIBLE_DEVICES": "0,1",
			},
			want: JobContext{
				JobID: "222",
				GPUs:  "0,1",
			},
		},
		{
			name: "SLURM_STEP_GPUS takes priority over CUDA_VISIBLE_DEVICES",
			env: map[string]string{
				"SLURM_JOB_ID":         "333",
				"SLURM_STEP_GPUS":      "2,3",
				"CUDA_VISIBLE_DEVICES": "0,1,2,3",
			},
			want: JobContext{
				JobID: "333",
				GPUs:  "2,3",
			},
		},
		{
			name: "SLURM_JOB_GPUS fallback",
			env: map[string]string{
				"SLURM_JOB_ID":   "444",
				"SLURM_JOB_GPUS": "0,1",
			},
			want: JobContext{
				JobID: "444",
				GPUs:  "0,1",
			},
		},
		{
			name: "GPU_DEVICE_ORDINAL fallback",
			env: map[string]string{
				"SLURM_JOB_ID":       "555",
				"GPU_DEVICE_ORDINAL": "3",
			},
			want: JobContext{
				JobID: "555",
				GPUs:  "3",
			},
		},
		{
			name: "bad int values default to zero",
			env: map[string]string{
				"SLURM_JOB_ID":        "666",
				"SLURM_JOB_NUM_NODES": "not-a-number",
				"SLURM_NTASKS":        "",
				"SLURM_PROCID":        "abc",
				"SLURM_LOCALID":       "-",
			},
			want: JobContext{
				JobID: "666",
			},
		},
		{
			name: "single node single gpu sbatch",
			env: map[string]string{
				"SLURM_JOB_ID":        "777",
				"SLURM_JOB_NAME":      "inference",
				"SLURM_JOB_PARTITION": "gpu",
				"SLURM_NODELIST":      "gpu-node-01",
				"SLURM_NODENAME":      "gpu-node-01",
				"SLURM_JOB_NUM_NODES": "1",
				"SLURM_NTASKS":        "1",
				"SLURM_PROCID":        "0",
				"SLURM_LOCALID":       "0",
				"SLURM_STEP_GPUS":     "0",
			},
			want: JobContext{
				JobID:     "777",
				JobName:   "inference",
				Partition: "gpu",
				NodeList:  "gpu-node-01",
				NodeName:  "gpu-node-01",
				NumNodes:  1,
				NumTasks:  1,
				ProcID:    0,
				LocalID:   0,
				GPUs:      "0",
			},
		},
		{
			name: "8-gpu DGX node",
			env: map[string]string{
				"SLURM_JOB_ID":        "888",
				"SLURM_JOB_NAME":      "megatron-lm",
				"SLURM_JOB_PARTITION": "dgx-a100",
				"SLURM_NODELIST":      "dgx[001-008]",
				"SLURM_NODENAME":      "dgx003",
				"SLURM_JOB_NUM_NODES": "8",
				"SLURM_NTASKS":        "64",
				"SLURM_PROCID":        "16",
				"SLURM_LOCALID":       "0",
				"SLURM_STEP_GPUS":     "0,1,2,3,4,5,6,7",
			},
			want: JobContext{
				JobID:     "888",
				JobName:   "megatron-lm",
				Partition: "dgx-a100",
				NodeList:  "dgx[001-008]",
				NodeName:  "dgx003",
				NumNodes:  8,
				NumTasks:  64,
				ProcID:    16,
				LocalID:   0,
				GPUs:      "0,1,2,3,4,5,6,7",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			setEnv(t, tt.env)
			got := FromEnv()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSlurmGPUPriority(t *testing.T) {
	// verify the full priority chain: STEP > JOB > ORDINAL > CUDA
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "step wins over everything",
			env: map[string]string{
				"SLURM_STEP_GPUS":      "0",
				"SLURM_JOB_GPUS":       "0,1",
				"GPU_DEVICE_ORDINAL":   "0,1,2",
				"CUDA_VISIBLE_DEVICES": "0,1,2,3",
			},
			want: "0",
		},
		{
			name: "job wins when no step",
			env: map[string]string{
				"SLURM_JOB_GPUS":       "0,1",
				"GPU_DEVICE_ORDINAL":   "0,1,2",
				"CUDA_VISIBLE_DEVICES": "0,1,2,3",
			},
			want: "0,1",
		},
		{
			name: "ordinal wins when no step or job",
			env: map[string]string{
				"GPU_DEVICE_ORDINAL":   "0,1,2",
				"CUDA_VISIBLE_DEVICES": "0,1,2,3",
			},
			want: "0,1,2",
		},
		{
			name: "cuda is last resort",
			env: map[string]string{
				"CUDA_VISIBLE_DEVICES": "4,5",
			},
			want: "4,5",
		},
		{
			name: "nothing set",
			env:  map[string]string{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			setEnv(t, tt.env)
			ctx := FromEnv()
			assert.Equal(t, tt.want, ctx.GPUs)
		})
	}
}

func TestVisibleDevices(t *testing.T) {
	tests := []struct {
		name string
		gpus string
		want []int
	}{
		{name: "multi gpu", gpus: "0,1,2,3", want: []int{0, 1, 2, 3}},
		{name: "single gpu", gpus: "2", want: []int{2}},
		{name: "empty", gpus: "", want: nil},
		{name: "with spaces", gpus: "0, 1, 3", want: []int{0, 1, 3}},
		{name: "non-numeric skipped", gpus: "0,gpu1,2", want: []int{0, 2}},
		{name: "all garbage", gpus: "foo,bar", want: []int{}},
		{name: "trailing comma", gpus: "0,1,", want: []int{0, 1}},
		{name: "high indices", gpus: "4,5,6,7", want: []int{4, 5, 6, 7}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := JobContext{GPUs: tt.gpus}
			assert.Equal(t, tt.want, j.VisibleDevices())
		})
	}
}

func TestHeaderRowAlignment(t *testing.T) {
	j := JobContext{
		JobID:   "100",
		JobName: "test",
		ProcID:  3,
	}
	assert.Equal(t, len(j.Header()), len(j.Row()))
}

func TestRowValues(t *testing.T) {
	j := JobContext{
		JobID:     "42",
		JobName:   "train",
		Partition: "gpu",
		NodeName:  "n01",
		ProcID:    5,
		LocalID:   1,
		GPUs:      "0,1",
	}
	row := j.Row()
	assert.Equal(t, "42", row[0])
	assert.Equal(t, "train", row[1])
	assert.Equal(t, "gpu", row[2])
	assert.Equal(t, "n01", row[3])
	assert.Equal(t, "5", row[4])
	assert.Equal(t, "1", row[5])
	assert.Equal(t, "0,1", row[6])
}

func TestRowZeroValues(t *testing.T) {
	j := JobContext{}
	row := j.Row()
	// ProcID and LocalID should be "0", not empty
	assert.Equal(t, "0", row[4])
	assert.Equal(t, "0", row[5])
}

func TestHeaderContents(t *testing.T) {
	j := JobContext{}
	hdr := j.Header()
	assert.Contains(t, hdr, "JobID")
	assert.Contains(t, hdr, "Node")
	assert.Contains(t, hdr, "GPUs")
	assert.Contains(t, hdr, "Rank")
}
