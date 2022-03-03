// Copyright 2020 Mia srl
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
	"os"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const overlay string = "overlayTesting/"

func createTestFiles(fs afero.Fs, files []string) error {
	for _, file := range files {
		_, err := fs.Create(overlay + file)
		if err != nil {
			return err
		}
	}
	return nil
}

func TestFindFiles(t *testing.T) {

	testCases := []struct {
		desc     string
		input    []string
		expected map[fileType][]string
	}{
		{
			desc:  "only resources and patch yml",
			input: []string{"default.certificate.yml", "mlp.yml", "deployment1.patch.yml"},
			expected: map[fileType][]string{
				Patch:    {"deployment1.patch.yml"},
				Resource: {"default.certificate.yml", "mlp.yml"},
			},
		},
		{
			desc:  "yaml yml and others extentions and kustomization.yaml",
			input: []string{"kustomization.yaml", "default.certificate.yaml", "mlp.yml", "deployment1.patch.xyz"},
			expected: map[fileType][]string{
				Resource: {"default.certificate.yaml", "mlp.yml"},
			},
		},
		{
			desc:  "some char in filename uppercase",
			input: []string{"DEFAULT.certificate.yaml", "mlp.YML", "deployment1.PATCH.yml"},
			expected: map[fileType][]string{
				Resource: {"DEFAULT.certificate.yaml", "mlp.YML"},
				Patch:    {"deployment1.PATCH.yml"},
			},
		},
		{
			desc:  "patch in resource name",
			input: []string{"mypatchresource.deployment.yml"},
			expected: map[fileType][]string{
				Resource: {"mypatchresource.deployment.yml"},
			},
		},
		{
			desc:  "patch with both extentions",
			input: []string{"myresource.patch.yml", "myresource.patch.yaml"},
			expected: map[fileType][]string{
				Patch: {"myresource.patch.yaml", "myresource.patch.yml"},
			},
		},
		{
			desc:  "only patch.yaml",
			input: []string{"patch.yaml", "patch.yml", "aaapatch.yaml", "aaapatch.yml"},
			expected: map[fileType][]string{
				Patch:    {"patch.yaml", "patch.yml"},
				Resource: {"aaapatch.yaml", "aaapatch.yml"},
			},
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			createTestFiles(fs, tC.input)
			actual, err := findFiles(fs, overlay)
			require.Nil(t, err)
			require.Equal(t, tC.expected, actual)
		})
	}

}

func setup(t *testing.T, fsys filesys.FileSystem) filesys.ConfirmedDir {
	dir, err := filesys.NewTmpConfirmedDir()
	require.Nil(t, err)
	fsys.Mkdir(dir.Join(overlay))
	fsys.Create(dir.Join(overlay + "kustomization.yaml"))
	fsys.Create(dir.Join(overlay + "resource1.service.yaml"))
	return dir
}

func TestExecAdd(t *testing.T) {
	fsys := filesys.MakeFsOnDisk()

	testCases := []struct {
		desc      string
		input     map[fileType][]string
		check     func(*testing.T, filesys.ConfirmedDir)
		kustomize string
	}{
		{
			desc: "patch and resources",
			input: map[fileType][]string{
				Patch:    {"deployment1.PATCH.yml"},
				Resource: {"resource1.service.yaml"},
			},
			check: func(t *testing.T, dir filesys.ConfirmedDir) {
				kustomization, err := fsys.ReadFile(dir.Join(overlay + "kustomization.yaml"))
				require.Nil(t, err)
				require.YAMLEq(t, "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment1.PATCH.yml\nresources:\n- resource1.service.yaml", string(kustomization))
			},
			kustomize: "",
		},
		{
			desc: "already present patch in kustomize.yaml",
			input: map[fileType][]string{
				Patch:    {"deployment1.PATCH.yml"},
				Resource: {"resource1.service.yaml"},
			},
			check: func(t *testing.T, dir filesys.ConfirmedDir) {
				kustomization, err := fsys.ReadFile(dir.Join(overlay + "kustomization.yaml"))
				require.Nil(t, err)
				require.YAMLEq(t, "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment1.PATCH.yml\nresources:\n- resource1.service.yaml", string(kustomization))
			},
			kustomize: "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment1.PATCH.yml\n",
		},
	}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			dir := setup(t, fsys)
			defer os.RemoveAll(dir.String())
			if tC.kustomize != "" {
				err := fsys.WriteFile(dir.Join(overlay+"kustomization.yaml"), []byte(tC.kustomize))
				require.Nil(t, err)
			}
			err := execAdd(fsys, dir.Join(overlay), tC.input)
			require.Nil(t, err)
			tC.check(t, dir)
		})
	}
}

func TestFilterPatchFiles(t *testing.T) {
	testCases := []struct {
		desc          string
		expected      []string
		input         []string
		kustomization string
	}{
		{
			desc:          "some res in kustomization.yaml",
			expected:      []string{"res-not-present.yaml"},
			input:         []string{"resource1.service.yaml", "res-not-present.yaml"},
			kustomization: "apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\npatches:\n- path: deployment1.PATCH.yml\nresources:\n- resource1.service.yaml",
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			fsys := filesys.MakeFsInMemory()
			err := fsys.WriteFile("kustomization.yaml", []byte(tC.kustomization))
			require.Nil(t, err)
			actual, err := filterPatchFiles(tC.input, fsys)
			require.Nil(t, err)
			require.Equal(t, tC.expected, actual)
		})
	}
}
