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
	"errors"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
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

func TestCommandWithFlags(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args        []string
		expectError bool
	}{
		"with load-restrictor none": {
			args: []string{"--load-restrictor", "none", "testdata"},
		},
		"with enable-helm": {
			args: []string{"--enable-helm", "testdata"},
		},
		"with output flag": {
			args: []string{"-o", filepath.Join(t.TempDir(), "out.yaml"), "testdata"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cmd := NewCommand()
			buffer := new(bytes.Buffer)
			cmd.SetArgs(test.args)
			cmd.SetOut(buffer)
			cmd.SetErr(buffer)
			err := cmd.Execute()
			if test.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAddFlags(t *testing.T) {
	t.Parallel()

	flags := &Flags{}
	set := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.AddFlags(set)

	// verify defaults
	assert.Empty(t, flags.outputPath)
	assert.False(t, flags.enableHelm)
	assert.Equal(t, "helm", flags.helmCommand)
	assert.Nil(t, flags.helmAPIVersions)
	assert.Empty(t, flags.helmKubeVersion)
	assert.Equal(t, "rootOnly", flags.loadRestrictor)

	// verify parsing
	err := set.Parse([]string{
		"--enable-helm",
		"--helm-command", "/usr/local/bin/helm",
		"--helm-api-versions", "v1,apps/v1",
		"--helm-kube-version", "1.30.0",
		"--load-restrictor", "none",
		"-o", "/tmp/out.yaml",
	})
	require.NoError(t, err)
	assert.True(t, flags.enableHelm)
	assert.Equal(t, "/usr/local/bin/helm", flags.helmCommand)
	assert.Equal(t, []string{"v1", "apps/v1"}, flags.helmAPIVersions)
	assert.Equal(t, "1.30.0", flags.helmKubeVersion)
	assert.Equal(t, "none", flags.loadRestrictor)
	assert.Equal(t, "/tmp/out.yaml", flags.outputPath)
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
		"all fields propagated": {
			flags: &Flags{
				outputPath:      filepath.Join(testPath, "file.txt"),
				enableHelm:      true,
				helmCommand:     "/usr/local/bin/helm",
				helmAPIVersions: []string{"v1", "apps/v1"},
				helmKubeVersion: "1.30.0",
				loadRestrictor:  "none",
			},
			args: []string{"input"},
			expectedOptions: &Options{
				outputPath:      filepath.Join(testPath, "file.txt"),
				inputPath:       "input",
				enableHelm:      true,
				helmCommand:     "/usr/local/bin/helm",
				helmAPIVersions: []string{"v1", "apps/v1"},
				helmKubeVersion: "1.30.0",
				loadRestrictor:  "none",
				fSys:            fSys,
				writer:          buffer,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

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
	t.Parallel()

	tests := map[string]struct {
		options        *Options
		expectedOutput string
		expectedError  string
	}{
		"run correctly": {
			options: &Options{
				inputPath:      "testdata",
				outputPath:     "",
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				loadRestrictor: "rootOnly",
			},
		},
		"run saving on file": {
			options: &Options{
				inputPath:      "testdata",
				outputPath:     filepath.Join(t.TempDir(), "output.yaml"),
				fSys:           filesys.MakeFsOnDisk(),
				loadRestrictor: "rootOnly",
			},
		},
		"error reading files": {
			options: &Options{
				inputPath:      "testdata",
				outputPath:     "",
				fSys:           filesys.MakeEmptyDirInMemory(),
				loadRestrictor: "rootOnly",
			},
			expectedError: "not a valid directory",
		},
		"error during writes": {
			options: &Options{
				inputPath:      "testdata",
				outputPath:     "",
				fSys:           filesys.MakeFsOnDisk(),
				writer:         failWriter{},
				loadRestrictor: "rootOnly",
			},
			expectedError: "nope",
		},
		"run with load restrictor none": {
			options: &Options{
				inputPath:      "testdata",
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				loadRestrictor: "none",
			},
		},
		"error for invalid load restrictor": {
			options: &Options{
				inputPath:      "testdata",
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				loadRestrictor: "invalid",
			},
			expectedError: `invalid load-restrictor value "invalid"`,
		},
		"error for empty load restrictor": {
			options: &Options{
				inputPath:      "testdata",
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				loadRestrictor: "",
			},
			expectedError: `invalid load-restrictor value ""`,
		},
		"enable helm without charts is no-op": {
			options: &Options{
				inputPath:      "testdata",
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				enableHelm:     true,
				helmCommand:    "helm",
				loadRestrictor: "rootOnly",
			},
		},
		"cross-directory with rootOnly fails": {
			options: &Options{
				inputPath:      filepath.Join("testdata", "cross-directory"),
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				loadRestrictor: "rootOnly",
			},
			expectedError: "is not in or below",
		},
		"cross-directory with none succeeds": {
			options: &Options{
				inputPath:      filepath.Join("testdata", "cross-directory"),
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				loadRestrictor: "none",
			},
			expectedOutput: "shared-config",
		},
		"with-resources produces output": {
			options: &Options{
				inputPath:      filepath.Join("testdata", "with-resources"),
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				loadRestrictor: "rootOnly",
			},
			expectedOutput: "test-config",
		},
		"helm chart inflation with fake helm": {
			options: &Options{
				inputPath:      filepath.Join("testdata", "with-helm"),
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				enableHelm:     true,
				helmCommand:    fakeHelmPath(t),
				loadRestrictor: "rootOnly",
			},
			expectedOutput: "from-helm-chart",
		},
		"helm chart inflation disabled fails": {
			options: &Options{
				inputPath:      filepath.Join("testdata", "with-helm"),
				fSys:           filesys.MakeFsOnDisk(),
				writer:         new(bytes.Buffer),
				enableHelm:     false,
				loadRestrictor: "rootOnly",
			},
			expectedError: "must specify --enable-helm",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			err := test.options.Run(t.Context())
			switch len(test.expectedError) {
			case 0:
				assert.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}

			if test.expectedOutput != "" {
				if buf, ok := test.options.writer.(*bytes.Buffer); ok {
					assert.Contains(t, buf.String(), test.expectedOutput)
				}
			}
		})
	}
}

// FailWriter is a writer that always returns an error.
type failWriter struct{}

// Write implements the Writer interface's Write method and returns an error.
func (failWriter) Write([]byte) (int, error) { return 0, errors.New("nope") }

func fakeHelmPath(t *testing.T) string {
	t.Helper()
	absPath, err := filepath.Abs(filepath.Join("testdata", "with-helm", "fake-helm.sh"))
	require.NoError(t, err)
	return absPath
}
