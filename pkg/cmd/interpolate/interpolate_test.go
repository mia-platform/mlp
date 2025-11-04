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

package interpolate

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()
	assert.NotNil(t, cmd)
}

func TestOptions(t *testing.T) {
	t.Parallel()

	buffer := new(bytes.Buffer)
	fSys := filesys.MakeEmptyDirInMemory()
	expectedOpts := &Options{
		prefixes:   []string{"prefix"},
		inputPaths: []string{"input"},
		outputPath: "output",
		fSys:       fSys,
		reader:     buffer,
	}

	flag := &Flags{
		prefixes:   []string{"prefix"},
		inputPaths: []string{"input"},
		outputPath: "output",
	}
	opts, err := flag.ToOptions(buffer, fSys)
	require.NoError(t, err)

	assert.Equal(t, expectedOpts, opts)
	assert.NoError(t, opts.Validate())
	opts.inputPaths = []string{}

	assert.ErrorContains(t, opts.Validate(), "at least one path must be specified")

	opts.inputPaths = []string{"input", stdinToken}
	assert.ErrorContains(t, opts.Validate(), "cannot read from stdin and other paths together")
}

func TestRun(t *testing.T) {
	testdata := "testdata"

	t.Setenv("MLP_SIMPLE_ENV", "test")
	t.Setenv("MLP_DOLLAR_ENV", "$contains$dollars$")
	t.Setenv("MLP_JSON_MULTILINE_ENV", `{
    "first": "field",
    "second": "field",
    "third": "field",
    "fourth": "field"
  }`)
	t.Setenv("MLP_JSON_SINGLELINE_ENV", `{"type":"type","project_id":"id","private_key_id":"key","private_key":"-----BEGIN CERTIFICATE-----\nXXXXXXXXXXXXXXXXXXXXXXXXX\nYYYYYYYYYYYYYYY/4YYYYYYYYY\n-----END CERTIFICATE-----\n","client_email":"email@example.com","client_id":"client-id","auth_uri":"https://example.com/auth","token_uri":"https://example.com/token","auth_provider_x509_cert_url":"https://example.com/certs","client_x509_cert_url":"https://example.com/certs/fooo%40bar"}`)
	t.Setenv("MLP_STRING_ESCAPED_ENV", `abd\def`)
	t.Setenv("MLP_JSON_ESCAPED_ENV", `{ \"foo\": \"bar\" }`)
	t.Setenv("MLP_SPECIAL_JSON_ENV", `{ "foo": "bar\ntaz" }`)
	t.Setenv("MLP_TEST_STRING_ESCAPED_ENV", `env\\first\line`)
	t.Setenv("MLP_MULTILINE_STRING_ESCAPED_ENV", "env\\\\first\\line\nenv\tsecondline\nenvthirdline\n")
	t.Setenv("MLP_NUMBER_ENV", `4`)
	t.Setenv("MLP_MULTILINE_STRING", `-----BEGIN CERTIFICATE-----
XXXXXXXXXXXXXXXXXXXXXXXXX
YYYYYYYYYYYYYYY/4YYYYYYYYY
ZZZZZZZZZZZZZZZZZZZZZZZZZZ
-----END CERTIFICATE-----`)
	t.Setenv("MLP_HTML", "env with spaces and \"")

	fSys := filesys.MakeFsOnDisk()
	testTmpDir := t.TempDir()
	tests := map[string]struct {
		option              *Options
		expectedError       string
		expectedResultsPath string
	}{
		"interpolate multiple paths": {
			option: &Options{
				prefixes:   []string{"MLP_TEST_", "MLP_"},
				inputPaths: []string{filepath.Join(testdata, "folder"), filepath.Join(testdata, "file.yaml")},
				outputPath: filepath.Join(testTmpDir, "outputs-multiple-paths"),
				fSys:       fSys,
				reader:     new(bytes.Buffer),
			},
			expectedResultsPath: filepath.Join(testdata, "results"),
		},
		"interpolate with multiple paths contained inside each others": {
			option: &Options{
				prefixes:   []string{"MLP_TEST_", "MLP_"},
				inputPaths: []string{filepath.Join(testdata, "folder"), filepath.Join(testdata, "folder", "inner")},
				outputPath: filepath.Join(testTmpDir, "outputs-multiple-contained-paths"),
				fSys:       fSys,
				reader:     new(bytes.Buffer),
			},
			expectedResultsPath: filepath.Join(testdata, "results-contained"),
		},
		"interpolate from reader": {
			option: &Options{
				prefixes:   []string{"MLP_"},
				inputPaths: []string{stdinToken},
				outputPath: filepath.Join(testTmpDir, "output-stdin"),
				fSys:       fSys,
				reader: func() io.Reader {
					data, err := fSys.ReadFile(filepath.Join(testdata, "file.yaml"))
					require.NoError(t, err)
					return bytes.NewReader(data)
				}(),
			},
			expectedResultsPath: filepath.Join(testdata, "stdin"),
		},
		"error with missing env": {
			option: &Options{
				prefixes:   []string{"MLP_MISSING"},
				inputPaths: []string{filepath.Join(testdata, "missing-env.yaml")},
				outputPath: filepath.Join(testTmpDir, "outputs-missing-envs"),
				fSys:       fSys,
			},
			expectedError: `environment variable "MISSING_ENV" not found`,
		},
		"error with ensuring output folder": {
			option: &Options{
				outputPath: func() string {
					tmpdir := filepath.Join(testTmpDir, "non-executable")
					require.NoError(t, os.MkdirAll(tmpdir, os.ModePerm))
					require.NoError(t, os.Chmod(tmpdir, 0444))
					return filepath.Join(tmpdir, "output")
				}(),
				fSys: fSys,
			},
			expectedError: "output: permission denied",
		},
		"error writing in output directory": {
			option: &Options{
				prefixes:   []string{"MLP_TEST_", "MLP_"},
				inputPaths: []string{filepath.Join(testdata, "file.yaml")},
				outputPath: func() string {
					tmpdir := filepath.Join(testTmpDir, "non-writable")
					require.NoError(t, os.MkdirAll(tmpdir, os.ModePerm))
					require.NoError(t, os.Chmod(tmpdir, 0555))
					return tmpdir
				}(),
				fSys:   fSys,
				reader: new(bytes.Buffer),
			},
			expectedError: "file.yaml: permission denied",
		},
		"error missing input folder": {
			option: &Options{
				prefixes:   []string{"MLP_TEST_", "MLP_"},
				inputPaths: []string{filepath.Join(testdata, "missing")},
				outputPath: filepath.Join(testTmpDir, "no-input"),
				fSys:       fSys,
				reader:     new(bytes.Buffer),
			},
			expectedError: "no such file or directory",
		},
		"allow missing filenames flag will skip path": {
			option: &Options{
				prefixes:              []string{"MLP_TEST_", "MLP_"},
				inputPaths:            []string{filepath.Join(testdata, "file.yaml"), filepath.Join(testdata, "missing")},
				allowMissingFilenames: true,
				outputPath:            filepath.Join(testTmpDir, "outputs-missing-path"),
				fSys:                  fSys,
				reader:                new(bytes.Buffer),
			},
			expectedResultsPath: filepath.Join(testdata, "results-missing-path"),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			err := test.option.Run(t.Context())

			switch len(test.expectedError) {
			case 0:
				assert.NoError(t, err)
				testStructure(t, test.option.outputPath, test.expectedResultsPath)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
		})
	}
}

func testStructure(t *testing.T, pathToTest, expectationPath string) {
	t.Helper()

	// walk both pathToTest and expectationPath to ensure they have the same structure and contents
	err := filepath.WalkDir(pathToTest, func(testPath string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		expectedPath := filepath.Join(expectationPath, strings.TrimPrefix(testPath, pathToTest))

		if d.IsDir() {
			require.DirExists(t, testPath)
		} else {
			data, err := os.ReadFile(testPath)
			require.NoError(t, err)
			expectedData, err := os.ReadFile(expectedPath)
			require.NoError(t, err)
			assert.Equal(t, string(expectedData), string(data), expectedPath)
		}

		return nil
	})
	require.NoError(t, err)

	err = filepath.WalkDir(expectationPath, func(expectedPath string, d fs.DirEntry, err error) error {
		require.NoError(t, err)
		testPath := filepath.Join(pathToTest, strings.TrimPrefix(expectedPath, expectationPath))

		if d.IsDir() {
			require.DirExists(t, testPath)
		} else {
			data, err := os.ReadFile(testPath)
			require.NoError(t, err)
			expectedData, err := os.ReadFile(expectedPath)
			require.NoError(t, err)
			assert.Equal(t, string(expectedData), string(data), testPath)
		}

		return nil
	})
	require.NoError(t, err)
}
