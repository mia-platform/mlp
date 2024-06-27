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
	"bytes"
	"fmt"
	"path/filepath"

	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	yaml "sigs.k8s.io/yaml/goyaml.v3"
)

type kustomizationFile struct {
	path string
	fSys filesys.FileSystem
}

// newKustomizationFile returns a new instance.
func newKustomizationFile(fSys filesys.FileSystem, path string) (*kustomizationFile, error) {
	kf := &kustomizationFile{fSys: fSys}
	err := kf.validate(path)
	if err != nil {
		return nil, err
	}
	return kf, nil
}

func (kf *kustomizationFile) GetPath() string {
	return kf.path
}

func (kf *kustomizationFile) validate(path string) error {
	match := 0
	var paths []string
	for _, kfilename := range konfig.RecognizedKustomizationFileNames() {
		fullpath := filepath.Join(path, kfilename)
		if kf.fSys.Exists(fullpath) {
			match++
			paths = append(paths, fullpath)
		}
	}

	switch match {
	case 0:
		return fmt.Errorf("missing kustomization file %q", konfig.DefaultKustomizationFileName())
	case 1:
		kf.path = paths[0]
	default:
		return fmt.Errorf("found multiple kustomization file: %v", path)
	}

	if kf.fSys.IsDir(kf.path) {
		return fmt.Errorf("%s should be a file", kf.path)
	}
	return nil
}

func (kf *kustomizationFile) Read() (*types.Kustomization, error) {
	data, err := kf.fSys.ReadFile(kf.path)
	if err != nil {
		return nil, err
	}

	var k types.Kustomization
	if err := k.Unmarshal(data); err != nil {
		return nil, err
	}

	k.FixKustomization()

	return &k, nil
}

func (kf *kustomizationFile) write(kustomization *types.Kustomization) error {
	data, err := kf.marshal(kustomization)
	if err != nil {
		return err
	}
	return kf.fSys.WriteFile(kf.path, data)
}

func (kf *kustomizationFile) marshal(k *types.Kustomization) ([]byte, error) {
	buffer := new(bytes.Buffer)
	encoder := yaml.NewEncoder(buffer)
	encoder.SetIndent(2)
	encoder.CompactSeqIndent()

	if err := encoder.Encode(k); err != nil {
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
