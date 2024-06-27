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
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

func TestNewCommand(t *testing.T) {
	t.Parallel()

	cmd := NewCommand()
	assert.NotNil(t, cmd)
	cmd.SetArgs([]string{"testdata"})
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
resources:
- Test4.YAML
- test1.yaml
- test2.yml
`
	expectedOverlaysData := `kind: Kustomization
apiVersion: kustomize.config.k8s.io/v1beta1
patches:
- path: path.yaml
- path: test4.patch.yml
- path: test2.patch.YAML
resources:
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
			err := options.Run(context.TODO())
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
	err = fSys.WriteFile(filepath.Join(overlaysFolder, "kustomization.yml"), []byte(`kind: Kustomization
apiVersion: kustomize.config.k8s.io/v1beta1
patches:
- path: path.yaml
- path: test4.patch.yml
`))
	require.NoError(t, err)

	return fSys
}

// func TestFindFiles(t *testing.T) {
// 	testCases := []struct {
// 		desc     string
// 		input    []string
// 		expected map[fileType][]string
// 	}{
// 		{
// 			desc:  "only resources and patch yml",
// 			input: []string{"default.certificate.yml", "mlp.yml", "deployment1.patch.yml"},
// 			expected: map[fileType][]string{
// 				Patch:    {"deployment1.patch.yml"},
// 				Resource: {"default.certificate.yml", "mlp.yml"},
// 			},
// 		},
// 		{
// 			desc:  "yaml yml and others extensions and kustomization.yaml",
// 			input: []string{"kustomization.yaml", "default.certificate.yaml", "mlp.yml", "deployment1.patch.xyz"},
// 			expected: map[fileType][]string{
// 				Resource: {"default.certificate.yaml", "mlp.yml"},
// 			},
// 		},
// 		{
// 			desc:  "some char in filename uppercase",
// 			input: []string{"DEFAULT.certificate.yaml", "mlp.YML", "deployment1.PATCH.yml"},
// 			expected: map[fileType][]string{
// 				Resource: {"DEFAULT.certificate.yaml", "mlp.YML"},
// 				Patch:    {"deployment1.PATCH.yml"},
// 			},
// 		},
// 		{
// 			desc:  "patch in resource name",
// 			input: []string{"mypatchresource.deployment.yml"},
// 			expected: map[fileType][]string{
// 				Resource: {"mypatchresource.deployment.yml"},
// 			},
// 		},
// 		{
// 			desc:  "patch with both extensions",
// 			input: []string{"myresource.patch.yml", "myresource.patch.yaml"},
// 			expected: map[fileType][]string{
// 				Patch: {"myresource.patch.yaml", "myresource.patch.yml"},
// 			},
// 		},
// 		{
// 			desc:  "only patch.yaml",
// 			input: []string{"patch.yaml", "patch.yml", "aaapatch.yaml", "aaapatch.yml"},
// 			expected: map[fileType][]string{
// 				Patch:    {"patch.yaml", "patch.yml"},
// 				Resource: {"aaapatch.yaml", "aaapatch.yml"},
// 			},
// 		},
// 	}
// 	for _, tC := range testCases {
// 		t.Run(tC.desc, func(t *testing.T) {
// 			fs := afero.NewMemMapFs()
// 			createTestFiles(fs, tC.input)
// 			actual, err := findFiles(fs, overlay)
// 			require.Nil(t, err)
// 			require.Equal(t, tC.expected, actual)
// 		})
// 	}
// }

// func setup(t *testing.T, fsys filesys.FileSystem) filesys.ConfirmedDir {
// 	t.Helper()
// 	dir, err := filesys.NewTmpConfirmedDir()
// 	require.Nil(t, err)
// 	fsys.Mkdir(dir.Join(overlay))
// 	fsys.Create(dir.Join(overlay + "kustomization.yaml"))
// 	fsys.Create(dir.Join(overlay + "resource1.service.yaml"))
// 	return dir
// }

// func TestExecAdd(t *testing.T) {
// 	fsys := filesys.MakeFsOnDisk()

// 	testCases := []struct {
// 		desc      string
// 		input     map[fileType][]string
// 		check     func(*testing.T, filesys.ConfirmedDir)
// 		kustomize string
// 	}{
// 		{
// 			desc: "patch and resources",
// 			input: map[fileType][]string{
// 				Patch:    {"deployment1.PATCH.yml"},
// 				Resource: {"resource1.service.yaml"},
// 			},
// 			check: func(t *testing.T, dir filesys.ConfirmedDir) {
// 				t.Helper()
// 				kustomization, err := fsys.ReadFile(dir.Join(overlay + "kustomization.yaml"))
// 				require.Nil(t, err)
// 				require.YAMLEq(t, "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment1.PATCH.yml\nresources:\n- resource1.service.yaml", string(kustomization))
// 			},
// 			kustomize: "",
// 		},
// 		{
// 			desc: "already present patch in kustomize.yaml",
// 			input: map[fileType][]string{
// 				Patch:    {"deployment1.PATCH.yml"},
// 				Resource: {"resource1.service.yaml"},
// 			},
// 			check: func(t *testing.T, dir filesys.ConfirmedDir) {
// 				t.Helper()
// 				kustomization, err := fsys.ReadFile(dir.Join(overlay + "kustomization.yaml"))
// 				require.Nil(t, err)
// 				require.YAMLEq(t, "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment1.PATCH.yml\nresources:\n- resource1.service.yaml", string(kustomization))
// 			},
// 			kustomize: "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment1.PATCH.yml\n",
// 		},
// 	}

// 	for _, tC := range testCases {
// 		t.Run(tC.desc, func(t *testing.T) {
// 			dir := setup(t, fsys)
// 			defer os.RemoveAll(dir.String())
// 			if tC.kustomize != "" {
// 				err := fsys.WriteFile(dir.Join(overlay+"kustomization.yaml"), []byte(tC.kustomize))
// 				require.Nil(t, err)
// 			}
// 			err := execAdd(fsys, dir.Join(overlay), tC.input)
// 			require.Nil(t, err)
// 			tC.check(t, dir)
// 		})
// 	}
// }

// func TestFilterPatchFiles(t *testing.T) {
// 	testCases := []struct {
// 		desc          string
// 		expected      []string
// 		input         []string
// 		kustomization string
// 	}{
// 		{
// 			desc:          "some res in kustomization.yaml",
// 			expected:      []string{"res-not-present.yaml"},
// 			input:         []string{"resource1.service.yaml", "res-not-present.yaml"},
// 			kustomization: "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment1.PATCH.yml\nresources:\n- resource1.service.yaml",
// 		},
// 	}
// 	for _, tC := range testCases {
// 		t.Run(tC.desc, func(t *testing.T) {
// 			fsys := filesys.MakeFsInMemory()
// 			err := fsys.WriteFile("kustomization.yaml", []byte(tC.kustomization))
// 			require.Nil(t, err)
// 			actual, err := filterPatchFiles(tC.input, fsys)
// 			require.Nil(t, err)
// 			require.Equal(t, tC.expected, actual)
// 		})
// 	}
// }

// func TestFilterPatchFilesWithMultilePatches(t *testing.T) {
// 	testCases := []struct {
// 		desc          string
// 		expected      []string
// 		input         []string
// 		kustomization string
// 	}{
// 		{
// 			desc:          "some res in kustomization.yaml",
// 			expected:      []string{"res-not-present.yaml"},
// 			input:         []string{"res-not-present.yaml", "deployment.PATCH.yml", "service.PATCH.yaml", "job.PATCH.yaml"},
// 			kustomization: "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment.PATCH.yml\n- path: service.PATCH.yaml\n- path: job.PATCH.yaml",
// 		},
// 	}
// 	for _, tC := range testCases {
// 		t.Run(tC.desc, func(t *testing.T) {
// 			fsys := filesys.MakeFsInMemory()
// 			err := fsys.WriteFile("kustomization.yaml", []byte(tC.kustomization))
// 			require.Nil(t, err)
// 			actual, err := filterPatchFiles(tC.input, fsys)
// 			require.Nil(t, err)
// 			require.Equal(t, tC.expected, actual)
// 		})
// 	}
// }
