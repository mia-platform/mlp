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

package resourceutil

import (
	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/yaml"
)

// Resource a resource reppresentation
type Resource struct {
	Filepath  string
	Name      string
	Head      ResourceHead
	Namespace string
	Info			*resource.Info
}

// ResourceHead the head of the resource
type ResourceHead struct {
	GroupVersion  string `json:"apiVersion"`
	Kind     string `json:"kind,omitempty"`
	Metadata *struct {
		Name        string            `json:"name"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata,omitempty"`
}

// NewResource create a new Resource from a file at `filepath`
// does NOT support multiple documents inside a single file
func NewResource(filepath string) (*Resource, error) {
	data, err := utils.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	var head ResourceHead
	if err := yaml.Unmarshal([]byte(data), &head); err != nil {
		return nil, err
	}

	return &Resource{
		Filepath: filepath,
		Name:     head.Metadata.Name,
		Head:     head,
	}, nil
}
