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
	"path/filepath"
	"testing"

	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNewDeployMutator(t *testing.T) {
	t.Parallel()

	mutator := NewDeployMutator(deployAll, true, "identifier")
	assert.NotNil(t, mutator)
}

func TestDeployMutatorCanHandleResource(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		obj            *metav1.PartialObjectMetadata
		expectedResult bool
	}{
		"config map is not handled": {
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       configMapGK.Kind,
					APIVersion: "v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: false,
		},
		"deployment return true": {
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       deployGK.Kind,
					APIVersion: "apps/v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: true,
		},
		"daemonset return true": {
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       dsGK.Kind,
					APIVersion: "apps/v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: true,
		},
		"stateful return true": {
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       stsGK.Kind,
					APIVersion: "apps/v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: true,
		},
		"pod return true": {
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       podGK.Kind,
					APIVersion: "v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := deployMutator{}
			assert.Equal(t, test.expectedResult, m.CanHandleResource(test.obj))
		})
	}
}

func TestDeployMutatorMutate(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "deploy-mutator")

	tests := map[string]struct {
		resource       *unstructured.Unstructured
		deployType     string
		forceNoSemver  bool
		expectedResult *unstructured.Unstructured
		expectedError  string
	}{
		"deployment": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
			deployType:     deployAll,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-deployment.yaml")),
		},
		"sts": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "sts.yaml")),
			deployType:     deploySmart,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-sts.yaml")),
		},
		"daemonset": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "daemonset.yaml")),
			deployType:     deploySmart,
			forceNoSemver:  true,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-daemonset.yaml")),
		},
		"pod": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "pod.yaml")),
			deployType:     deploySmart,
			forceNoSemver:  true,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-pod.yaml")),
		},
		"deployment smart deploy": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment-smart.yaml")),
			deployType:     deploySmart,
			forceNoSemver:  true,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-deployment-smart.yaml")),
		},
		"wrong resource": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-resource.yaml")),
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-resource.yaml")),
			expectedError:  `unsupported object type for dependencies mutator: "v1, Service"`,
		},
		"wrong image string": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-image.yaml")),
			deployType:     deploySmart,
			forceNoSemver:  true,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-image.yaml")),
			expectedError:  `couldn't parse image name "busybox:sha256:5be7104a4306abe768359a5379e6050ef69a29e9a5f99fcf7f46d5f7e9ba29a2"`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mutator := &deployMutator{
				deployType:    test.deployType,
				forceNoSemver: test.forceNoSemver,
				identifier:    "test-identifier",
			}

			err := mutator.Mutate(test.resource)
			switch len(test.expectedError) {
			case 0:
				require.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}

			assert.Equal(t, test.expectedResult, test.resource)
		})
	}
}
