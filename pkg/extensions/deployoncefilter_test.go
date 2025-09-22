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

package extensions

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/mia-platform/jpl/pkg/client/cache"
	"github.com/mia-platform/jpl/pkg/resource"
	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "filter")
	filtered := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "filtered.yaml"))

	tests := map[string]struct {
		object        *unstructured.Unstructured
		getter        cache.RemoteResourceGetter
		expected      bool
		expectedError string
	}{
		"no filtering if no config map or secret": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
		},
		"no filtering if config map use labels and not annotations": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "configmap.yaml")),
		},
		"no filtering if annotations with other value": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "secret.yaml")),
		},
		"filtering if annotation is present and remote object is found": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "filtered.yaml")),
			getter: &testGetter{
				availableObjects: map[resource.ObjectMetadata]*unstructured.Unstructured{
					resource.ObjectMetadataFromUnstructured(filtered): filtered,
				},
			},
			expected: true,
		},
		"no filtering if annotation is present but no remote object is found": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "filtered.yaml")),
			getter: &testGetter{
				availableObjects: map[resource.ObjectMetadata]*unstructured.Unstructured{},
			},
		},
		"error getting remote object": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "filtered.yaml")),
			getter: &testGetter{
				availableObjects: map[resource.ObjectMetadata]*unstructured.Unstructured{},
				errors: map[resource.ObjectMetadata]error{
					resource.ObjectMetadataFromUnstructured(filtered): errors.New("error on load"),
				},
			},
			expectedError: "error on load",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			filter := NewDeployOnceFilter()
			filtered, err := filter.Filter(test.object, test.getter)
			switch len(test.expectedError) {
			case 0:
				assert.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
			assert.Equal(t, test.expected, filtered)
		})
	}
}
