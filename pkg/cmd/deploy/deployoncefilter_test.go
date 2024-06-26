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

package deploy

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mia-platform/jpl/pkg/filter"
	inventoryfake "github.com/mia-platform/jpl/pkg/inventory/fake"
	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestFilter(t *testing.T) {
	t.Parallel()
	testdata := filepath.Join("testdata", "filter")
	filtered := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "filtered.yaml"))

	inventory := &inventoryfake.Inventory{InventoryObjects: []*unstructured.Unstructured{filtered}}
	testFilter := NewDeployOnceFilter(inventory)

	tests := map[string]struct {
		filter        filter.Interface
		object        *unstructured.Unstructured
		expected      bool
		expectedError string
	}{
		"no filtering if no config map or secret": {
			filter: testFilter,
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
		},
		"no filtering if config map use labels and not annotations": {
			filter: testFilter,
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "configmap.yaml")),
		},
		"no filtering if annotations with other value": {
			filter: testFilter,
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "secret.yaml")),
		},
		"filtering if annotation is present and in remote inventory": {
			filter:   testFilter,
			object:   jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "filtered.yaml")),
			expected: true,
		},
		"no filtering if annotation is present but not in remote inventory": {
			filter:   testFilter,
			object:   jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "filtered.yaml")),
			expected: true,
		},
		"error getting inventory": {
			filter:        NewDeployOnceFilter(&inventoryfake.Inventory{LoadErr: fmt.Errorf("error on load")}),
			object:        jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "filtered.yaml")),
			expectedError: "error on load",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			filtered, err := test.filter.Filter(test.object)
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
