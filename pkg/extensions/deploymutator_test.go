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
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/mia-platform/jpl/pkg/client/cache"
	"github.com/mia-platform/jpl/pkg/resource"
	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNewDeployMutator(t *testing.T) {
	t.Parallel()

	mutator := NewDeployMutator(DeployAll, true, "identifier")
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
		"statefulset return true": {
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
	remoteObject := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "remote-status.yaml"))
	remoteErrorObject := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "error-remote.yaml"))

	tests := map[string]struct {
		resource       *unstructured.Unstructured
		deployType     string
		forceNoSemver  bool
		expectedResult *unstructured.Unstructured
		expectedError  string
	}{
		"deployment": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
			deployType:     DeployAll,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-deployment.yaml")),
		},
		"sts": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "sts.yaml")),
			deployType:     DeploySmart,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-sts.yaml")),
		},
		"daemonset": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "daemonset.yaml")),
			deployType:     DeploySmart,
			forceNoSemver:  true,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-daemonset.yaml")),
		},
		"pod": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "pod.yaml")),
			deployType:     DeploySmart,
			forceNoSemver:  true,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-pod.yaml")),
		},
		"deployment smart deploy": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment-smart.yaml")),
			deployType:     DeploySmart,
			forceNoSemver:  true,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-deployment-smart.yaml")),
		},
		"deployment smart deploy with remote annotation": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment-smart-remote.yaml")),
			deployType:     DeploySmart,
			forceNoSemver:  true,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-deployment-smart-remote.yaml")),
		},
		"error getting resource from remote": {
			resource:       remoteErrorObject,
			deployType:     DeploySmart,
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "error-remote.yaml")),
			expectedError:  "error from remote",
		},
		"wrong resource": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-resource.yaml")),
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-resource.yaml")),
			expectedError:  `unsupported object type for dependencies mutator: "v1, Service"`,
		},
		"wrong image string": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-image.yaml")),
			deployType:     DeploySmart,
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

			getter := &testGetter{
				availableObjects: map[resource.ObjectMetadata]*unstructured.Unstructured{
					resource.ObjectMetadataFromUnstructured(remoteObject): remoteObject,
				},
				errors: map[resource.ObjectMetadata]error{
					resource.ObjectMetadataFromUnstructured(remoteErrorObject): fmt.Errorf("error from remote"),
				},
			}

			err := mutator.Mutate(test.resource, getter)
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

type testGetter struct {
	availableObjects map[resource.ObjectMetadata]*unstructured.Unstructured
	errors           map[resource.ObjectMetadata]error
}

func (g *testGetter) Get(_ context.Context, id resource.ObjectMetadata) (*unstructured.Unstructured, error) {
	if obj, found := g.availableObjects[id]; found {
		return obj, nil
	}

	if err, found := g.errors[id]; found {
		return nil, err
	}

	return nil, nil
}

var _ cache.RemoteResourceGetter = &testGetter{}
