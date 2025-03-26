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

package hydrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestNewCommand(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	file, err := os.Create(filepath.Join(tmpDir, "Kustomization"))
	require.NoError(t, err)
	require.NoError(t, file.Close())

	cmd := NewCommand()
	assert.NotNil(t, cmd)
	cmd.SetArgs([]string{tmpDir})
	assert.NoError(t, cmd.Execute())
}

func TestToOptions(t *testing.T) {
	t.Parallel()

	flags := &Flags{}
	fSys := filesys.MakeEmptyDirInMemory()
	o, err := flags.ToOptions(nil, fSys)
	require.NoError(t, err)
	assert.Equal(t, &Options{paths: []string{filesys.SelfDir}, fSys: fSys}, o)

	paths := []string{"one", "two"}
	o, err = flags.ToOptions(paths, fSys)
	require.NoError(t, err)
	assert.Equal(t, &Options{paths: paths, fSys: fSys}, o)
}

func TestRun(t *testing.T) {
	t.Parallel()

	expectedConfigurationData := `kind: Kustomization
apiVersion: kustomize.config.k8s.io/v1beta1
metadata:
  labels:
    app.kubernetes.io/managed-by: mlp
resources:
- Test4.YAML
- test1.yaml
- test2.yml
`
	expectedOverlaysData := `kind: Kustomization
apiVersion: kustomize.config.k8s.io/v1beta1
metadata:
  labels:
    app.kubernetes.io/managed-by: mlp
patches:
- path: path.yaml
  target:
    group: apps
    version: v1
    kind: Deployment
- path: test4.patch.yml
- path: config-file.yaml
- path: test2.patch.YAML
resources:
- test2.patch.yaml
- Test1.YAML
- test3.patches.YAML
`

	tests := map[string]struct {
		paths         []string
		expectedError string
	}{
		"run correctly": {
			paths: []string{"configuration", filepath.Join("overlays", "environment")},
		},
		"wrong path": {
			paths:         []string{"configurations"},
			expectedError: "'configurations' doesn't exist",
		},
		"missing kustomize file": {
			paths:         []string{"overlays"},
			expectedError: "missing kustomization file",
		},
		"multiple kustomization files": {
			paths:         []string{"multiple-files"},
			expectedError: "found multiple kustomization file",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fSys := testingInMemoryFSys(t)
			options := &Options{
				paths: test.paths,
				fSys:  fSys,
			}
			err := options.Run(t.Context())
			switch len(test.expectedError) {
			case 0:
				assert.NoError(t, err)
				data, err := fSys.ReadFile(filepath.Join("configuration", konfig.DefaultKustomizationFileName()))
				require.NoError(t, err)
				assert.Equal(t, expectedConfigurationData, string(data))
				data, err = fSys.ReadFile(filepath.Join("overlays", "environment", "kustomization.yml"))
				require.NoError(t, err)
				assert.Equal(t, expectedOverlaysData, string(data))
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
		})
	}
}

func testingInMemoryFSys(t *testing.T) filesys.FileSystem {
	t.Helper()
	overlaysFolder := filepath.Join("overlays", "environment")
	configurationFolder := "configuration"
	fSys := filesys.MakeEmptyDirInMemory()

	fSys.Mkdir("multiple-files")
	for _, kustomization := range konfig.RecognizedKustomizationFileNames() {
		err := fSys.WriteFile(filepath.Join("multiple-files", kustomization), []byte{})
		require.NoError(t, err)
	}

	fSys.MkdirAll(overlaysFolder)
	fSys.Mkdir(configurationFolder)
	err := fSys.WriteFile(filepath.Join(configurationFolder, "test1.yaml"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(configurationFolder, "test2.yml"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(configurationFolder, "test3.json"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(configurationFolder, "Test4.YAML"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(configurationFolder, konfig.DefaultKustomizationFileName()), []byte{})
	require.NoError(t, err)

	err = fSys.WriteFile(filepath.Join(overlaysFolder, "Test1.YAML"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(overlaysFolder, "test2.patch.YAML"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(overlaysFolder, "test3.patches.YAML"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(overlaysFolder, "test4.patch.yml"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(overlaysFolder, "config-file.yaml"), []byte{})
	require.NoError(t, err)
	err = fSys.WriteFile(filepath.Join(overlaysFolder, "kustomization.yml"), []byte(`kind: Kustomization
apiVersion: kustomize.config.k8s.io/v1beta1
resources:
- test2.patch.yaml
patches:
- path: path.yaml
  target:
    group: apps
    version: v1
    kind: Deployment
- path: test4.patch.yml
- path: config-file.yaml
`))
	require.NoError(t, err)

	return fSys
}
