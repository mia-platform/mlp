// Copyright Mia srl
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kustomize

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()
	assert.NotNil(t, cmd)

	buffer := new(bytes.Buffer)
	cmd.SetArgs([]string{"testdata"})
	cmd.SetOut(buffer)
	assert.NoError(t, cmd.Execute())
	t.Log(buffer.String())
	buffer.Reset()
}

func TestToOptions(t *testing.T) {
	t.Parallel()

	testPath := filepath.Join("test", "path")
	fSys := filesys.MakeEmptyDirInMemory()
	require.NoError(t, fSys.MkdirAll(testPath))
	buffer := new(bytes.Buffer)
	tests := map[string]struct {
		flags           *Flags
		args            []string
		expectedOptions *Options
		expectedError   string
	}{
		"create options from flags": {
			flags: &Flags{
				outputPath: filepath.Join(testPath, "file.txt"),
			},
			args: []string{"input"},
			expectedOptions: &Options{
				outputPath: filepath.Join(testPath, "file.txt"),
				inputPath:  "input",
				fSys:       fSys,
				writer:     buffer,
			},
		},
		"create options without args": {
			flags: &Flags{
				outputPath: filepath.Join(testPath, "file.txt"),
			},
			expectedOptions: &Options{
				outputPath: filepath.Join(testPath, "file.txt"),
				inputPath:  filesys.SelfDir,
				fSys:       fSys,
				writer:     buffer,
			},
		},
		"output path is a dir": {
			flags: &Flags{
				outputPath: testPath,
			},
			expectedError: outputIsADirectoryError,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			o, err := test.flags.ToOptions(test.args, fSys, buffer)
			switch len(test.expectedError) {
			case 0:
				assert.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}

			assert.Equal(t, test.expectedOptions, o)
		})
	}
}

func TestRun(t *testing.T) {
	tests := map[string]struct {
		options        *Options
		expectedOutput string
		expectedError  string
	}{
		"run correctly": {
			options: &Options{
				inputPath:  "testdata",
				outputPath: "",
				fSys:       filesys.MakeFsOnDisk(),
				writer:     new(bytes.Buffer),
			},
		},
		"run saving on file": {
			options: &Options{
				inputPath:  "testdata",
				outputPath: filepath.Join(t.TempDir(), "output.yaml"),
				fSys:       filesys.MakeFsOnDisk(),
			},
		},
		"error reading files": {
			options: &Options{
				inputPath:  "testdata",
				outputPath: "",
				fSys:       filesys.MakeEmptyDirInMemory(),
			},
			expectedError: "not a valid directory",
		},
		"error during writes": {
			options: &Options{
				inputPath:  "testdata",
				outputPath: "",
				fSys:       filesys.MakeFsOnDisk(),
				writer:     failWriter{},
			},
			expectedError: "nope",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.options.Run(t.Context())

			switch len(test.expectedError) {
			case 0:
				assert.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
		})
	}
}

// FailWriter is a writer that always returns an error.
type failWriter struct{}

// Write implements the Writer interface's Write method and returns an error.
func (failWriter) Write([]byte) (int, error) { return 0, fmt.Errorf("nope") }
