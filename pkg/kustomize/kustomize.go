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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/afero"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/kustomize/v4/commands/edit"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

type fileType int64

const (
	Patch fileType = iota
	Resource
)

func (s fileType) GetCommand() []string {
	switch s {
	case Patch:
		return []string{"add", "patch", "--path"}
	case Resource:
		return []string{"add", "resource"}
	}
	return []string{}
}

func findFiles(fs afero.Fs, overlay string) (map[fileType][]string, error) {
	output := make(map[fileType][]string)

	fileInfos, err := afero.ReadDir(fs, overlay)
	if err != nil {
		return nil, err
	}

	r := regexp.MustCompile(`(^|\.)patch\.ya?ml$`)
	for _, f := range fileInfos {
		name := strings.ToLower(f.Name())
		extension := filepath.Ext(name)
		if name == "kustomization.yaml" || (extension != ".yml" && extension != ".yaml") {
			continue
		}

		var t fileType

		if r.MatchString(name) { // if filename ends in .patch.yaml or .patch.yml regardless of case
			t = Patch
		} else {
			t = Resource
		}
		output[t] = append(output[t], f.Name())
	}

	return output, nil
}

// execute kustomize edit add command
func execAdd(fsys filesys.FileSystem, path string, allTypesFiles map[fileType][]string) error {
	pwd, err := os.Getwd()
	if err != nil {
		return err
	}
	defer func() {
		err = os.Chdir(pwd)
	}()

	err = os.Chdir(path)
	if err != nil {
		return err
	}

	// Cycle through each types of files ( patch, resource ) and for each file launch the correct edit command for that resource:
	// Patch: kustomize resource add patch --path f
	// Resource: kustomize resource add resource f
	for resType, files := range allTypesFiles {
		kustomizeCmd := resType.GetCommand()
		if resType == Patch {
			// Removes patches already present in kustomization.yaml
			files, err = filterPatchFiles(files, fsys)
			if err != nil {
				return err
			}
		}
		for _, f := range files {
			err := executeCmd(f, kustomizeCmd, fsys)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func filterPatchFiles(files []string, fsys filesys.FileSystem) ([]string, error) {
	// we are in the current directory
	fileCont, err := fsys.ReadFile("kustomization.yaml")
	if err != nil {
		return nil, fmt.Errorf("error reading kustomization.yaml")
	}

	var filesToAdd []string
	for _, f := range files {
		if !strings.Contains(string(fileCont), f) {
			filesToAdd = append(filesToAdd, f)
		}
	}
	return filesToAdd, nil
}

func executeCmd(file string, kustomizeCmd []string, fsys filesys.FileSystem) error {
	pvd := provider.NewDefaultDepProvider()
	editCmd := edit.NewCmdEdit(
		fsys, pvd.GetFieldValidator(), pvd.GetResourceFactory(), os.Stdout)
	editCmd.SetArgs(append(kustomizeCmd, file))
	err := editCmd.Execute()
	if err != nil {
		return err
	}
	return nil
}

// HydrateRun is the entrypoint to Hydrate command
func HydrateRun(paths []string) error {
	fs := afero.NewOsFs()
	fsys := filesys.MakeFsOnDisk()
	for _, path := range paths {
		files, err := findFiles(fs, path)
		if err != nil {
			return err
		}
		err = execAdd(fsys, path, files)
		if err != nil {
			return err
		}
	}
	return nil
}
