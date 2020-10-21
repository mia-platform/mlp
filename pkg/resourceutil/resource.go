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
	"errors"

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
	Info      *resource.Info
}

// ResourceHead the head of the resource
type ResourceHead struct {
	GroupVersion string `json:"apiVersion"`
	Kind         string `json:"kind,omitempty"`
	Metadata     *struct {
		Name        string            `json:"name"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata,omitempty"`
}

// InfoGenerator generate `resource.Info` given an input path
type InfoGenerator interface {
	Generate(path string) ([]*resource.Info, error)
}

// Builder wraps a `resource.Builder` and implements `InfoGenerator` interface
type Builder struct {
	builder *resource.Builder
}

// Generate use `resource.Builder` to generate a `resource.Info`
func (b *Builder) Generate(path string) ([]*resource.Info, error) {
	return b.builder.
		Path(false, path).
		Flatten().
		Do().Infos()
}

// NewResource create a new Resource from a file at `filepath`
// does NOT support multiple documents inside a single file
func NewResource(filepath string) (*Resource, error) {

	data, err := utils.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	var head ResourceHead
	if err = yaml.Unmarshal([]byte(data), &head); err != nil {
		return nil, err
	}

	return &Resource{
		Filepath: filepath,
		Name:     head.Metadata.Name,
		Head:     head,
	}, nil
}

// MakeInfo is the default function used to build `resource.Info`. It uses a builder to create
// the Infos starting from a YAML file path and then it set the correct namespace to the resource.
func MakeInfo(builder InfoGenerator, namespace string, path string) (*resource.Info, error) {
	infos, err := builder.Generate(path)
	if err != nil {
		return nil, err
	}

	if len(infos) != 1 {
		return nil, errors.New("Multiple objects in single yaml file currently not supported")
	}

	info := infos[0]
	info.Namespace = namespace
	return info, nil
}

// MakeResources creates a resource list and sorts them according to
// the standard ordering strategy
func MakeResources(opts *utils.Options, filePaths []string) ([]Resource, error) {

	resources := []Resource{}
	for _, path := range filePaths {

		// built every time because there is no mapping between `resourceutil.Resource`
		// and its corresponding `resource.Info`
		builder := &Builder{
			builder: resource.NewBuilder(opts.Config).
				Unstructured().
				RequireObject(true).
				Flatten(),
		}

		res, err := NewResource(path)
		if err != nil {
			return nil, err
		}
		res.Namespace = opts.Namespace
		info, err := MakeInfo(builder, res.Namespace, path)
		if err != nil {
			return nil, err
		}
		res.Info = info
		resources = append(resources, *res)
	}

	resources = SortResourcesByKind(resources, nil)
	return resources, nil
}
