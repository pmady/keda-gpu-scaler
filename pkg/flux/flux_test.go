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

package flux

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
			name: "inside flux job",
			env:  map[string]string{"FLUX_JOB_ID": "f23r45t"},
			want: true,
		},
		{
			name: "outside flux",
			env:  map[string]string{},
			want: false,
		},
		{
			name: "empty job id still counts",
			env:  map[string]string{"FLUX_JOB_ID": ""},
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
			name: "full flux environment",
			env: map[string]string{
				"FLUX_JOB_ID":          "f23r45t",
				"FLUX_TASK_RANK":       "4",
				"FLUX_TASK_LOCAL_ID":   "0",
				"FLUX_JOB_SIZE":        "8",
				"FLUX_JOB_NNODES":      "2",
				"FLUX_URI":             "local:///run/flux/local",
				"CUDA_VISIBLE_DEVICES": "0,1",
			},
			want: JobContext{
				JobID:    "f23r45t",
				TaskRank: 4,
				LocalID:  0,
				NumTasks: 8,
				NumNodes: 2,
				URI:      "local:///run/flux/local",
				GPUs:     "0,1",
			},
		},
		{
			name: "minimal - job id only",
			env: map[string]string{
				"FLUX_JOB_ID": "abc123",
			},
			want: JobContext{
				JobID: "abc123",
			},
		},
		{
			name: "empty env",
			env:  map[string]string{},
			want: JobContext{},
		},
		{
			name: "single gpu",
			env: map[string]string{
				"FLUX_JOB_ID":          "g99",
				"FLUX_TASK_RANK":       "0",
				"FLUX_TASK_LOCAL_ID":   "0",
				"FLUX_JOB_SIZE":        "1",
				"FLUX_JOB_NNODES":      "1",
				"CUDA_VISIBLE_DEVICES": "2",
			},
			want: JobContext{
				JobID:    "g99",
				TaskRank: 0,
				LocalID:  0,
				NumTasks: 1,
				NumNodes: 1,
				GPUs:     "2",
			},
		},
		{
			name: "8-gpu DGX node",
			env: map[string]string{
				"FLUX_JOB_ID":          "h100job",
				"FLUX_TASK_RANK":       "0",
				"FLUX_TASK_LOCAL_ID":   "0",
				"FLUX_JOB_SIZE":        "1",
				"FLUX_JOB_NNODES":      "1",
				"CUDA_VISIBLE_DEVICES": "0,1,2,3,4,5,6,7",
			},
			want: JobContext{
				JobID:    "h100job",
				TaskRank: 0,
				LocalID:  0,
				NumTasks: 1,
				NumNodes: 1,
				GPUs:     "0,1,2,3,4,5,6,7",
			},
		},
		{
			name: "multi-node MPI-style job",
			env: map[string]string{
				"FLUX_JOB_ID":          "mpirun42",
				"FLUX_TASK_RANK":       "16",
				"FLUX_TASK_LOCAL_ID":   "2",
				"FLUX_JOB_SIZE":        "64",
				"FLUX_JOB_NNODES":      "8",
				"CUDA_VISIBLE_DEVICES": "2,3",
			},
			want: JobContext{
				JobID:    "mpirun42",
				TaskRank: 16,
				LocalID:  2,
				NumTasks: 64,
				NumNodes: 8,
				GPUs:     "2,3",
			},
		},
		{
			name: "bad int values default to zero",
			env: map[string]string{
				"FLUX_JOB_ID":        "xyz",
				"FLUX_TASK_RANK":     "not-a-number",
				"FLUX_JOB_SIZE":      "",
				"FLUX_TASK_LOCAL_ID": "abc",
			},
			want: JobContext{
				JobID: "xyz",
			},
		},
		{
			name: "no CUDA_VISIBLE_DEVICES means no GPUs",
			env: map[string]string{
				"FLUX_JOB_ID":     "cpuonly",
				"FLUX_TASK_RANK":  "0",
				"FLUX_JOB_SIZE":   "4",
				"FLUX_JOB_NNODES": "1",
			},
			want: JobContext{
				JobID:    "cpuonly",
				NumTasks: 4,
				NumNodes: 1,
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
		{name: "mig uuid skipped", gpus: "GPU-abc123,1", want: []int{1}},
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
	j := JobContext{JobID: "test", TaskRank: 1}
	assert.Equal(t, len(j.Header()), len(j.Row()))
}

func TestRowValues(t *testing.T) {
	j := JobContext{
		JobID:    "f23r45t",
		TaskRank: 4,
		LocalID:  0,
		GPUs:     "0,1",
	}
	row := j.Row()
	assert.Equal(t, "f23r45t", row[0])
	assert.Equal(t, "4", row[1])
	assert.Equal(t, "0", row[2])
	assert.Equal(t, "0,1", row[3])
}

func TestRowZeroValues(t *testing.T) {
	j := JobContext{}
	row := j.Row()
	// TaskRank and LocalID should be "0", not empty
	assert.Equal(t, "0", row[1])
	assert.Equal(t, "0", row[2])
}

func TestHeaderContents(t *testing.T) {
	j := JobContext{}
	hdr := j.Header()
	assert.Contains(t, hdr, "FluxJobID")
	assert.Contains(t, hdr, "TaskRank")
	assert.Contains(t, hdr, "GPUs")
}
