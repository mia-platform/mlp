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
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const testdata = "testdata/"

func TestSortResourcesByKind(t *testing.T) {

	resources := []Resource{
		{Name: "a", Head: ResourceHead{Kind: "Unknown"}},
		{Name: "b", Head: ResourceHead{Kind: "Secret"}},
		{Name: "c", Head: ResourceHead{Kind: "ConfigMap"}},
		{Name: "d", Head: ResourceHead{Kind: "ClusterRole"}},
		{Name: "e", Head: ResourceHead{Kind: "IngressRoute"}},
		{Name: "f", Head: ResourceHead{Kind: "ClusterRoleBinding"}},
		{Name: "g", Head: ResourceHead{Kind: "ConfigMap"}},
		{Name: "h", Head: ResourceHead{Kind: "Deployment"}},
		{Name: "i", Head: ResourceHead{Kind: "PodSecurityPolicy"}},
		{Name: "j", Head: ResourceHead{Kind: "ServiceAccount"}},
		{Name: "k", Head: ResourceHead{Kind: "Service"}},
	}

	t.Run("Reordering resources based on default reordering function", func(t *testing.T) {
		expected := "ijdfbcgkhea"
		var orderedNames bytes.Buffer
		defer orderedNames.Reset()
		originalInput := resources
		for _, resource := range SortResourcesByKind(resources, nil) {
			orderedNames.WriteString(resource.Name)
		}

		if got := orderedNames.String(); got != expected {
			t.Errorf("Expected %q, got %q", expected, got)
		}

		for idx, resource := range originalInput {
			if resource != resources[idx] {
				t.Fatal("Expected input to SortResourcesByKind to stay the same")
			}
		}
	})
}

func TestNewResource(t *testing.T) {
	t.Run("Read a valid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "kubernetesersource.yaml")

		resource, err := NewResource(filePath)
		expectedMetadata := struct {
			Name        string            `json:"name"`
			Annotations map[string]string `json:"annotations"`
		}{
			Name: "literal",
		}
		expected := &Resource{
			Filepath: filePath,
			Name:     "literal",
			Head: ResourceHead{
				GroupVersion:  "v1",
				Kind:     "ConfigMap",
				Metadata: &expectedMetadata,
			},
		}
		require.Nil(t, err, "Reading a valid k8s file err must be nil")
		require.Equal(t, resource, expected, "Resource read from file must be equal to expected")
	})

	t.Run("Read an invalid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "notarresource.yaml")

		resource, err := NewResource(filePath)
		require.Nil(t, resource, "Reading an invalid k8s file resource must be nil")
		require.NotNil(t, err, "Reading an invalid k8s file resource an error must be returned")
	})
}
