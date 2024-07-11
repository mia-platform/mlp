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

func TestNewDependenciesMutator(t *testing.T) {
	t.Parallel()
	testdata := filepath.Join("testdata", "dependency-mutator")

	tests := map[string]struct {
		objects     []*unstructured.Unstructured
		expectedMap map[string]string
	}{
		"empty objects": {
			expectedMap: make(map[string]string),
		},
		"no secret or configmap in objects": {
			objects: []*unstructured.Unstructured{
				jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
			},
			expectedMap: make(map[string]string),
		},
		"mixed objects": {
			objects: []*unstructured.Unstructured{
				jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
				jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "configmap.yaml")),
				jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "secret.yaml")),
				jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "cm-other-namespace.yaml")),
			},
			expectedMap: map[string]string{
				"ConfigMapexampleother-ns":       "89557268706287308eefe82fbdbd327914ac22a23499a45be72ea79483d73a73",
				"ConfigMapexampleother-nsconfig": "efe6c20174d12ac077f3de985cae67175a6108ac084e813a7d0eb9c5ff9fe85d",
				"ConfigMapexampletest":           "474402695ca63dd67a8ee93690d46011d2e19181aeb10c616af3cb48ac36adad",
				"ConfigMapexampletestbconfig":    "f2ad8ef38c3f6fac4d0dcfa67696710c04bc88f92a54f6758cb43c0d392b3eea",
				"ConfigMapexampletestconfig":     "3f564266de9477b004c53c67de5eb4ec7cedb6dcee5b3d6d77ca2ed6cdd323ca",
				"Secretexampletest":              "f8ac3c753041ab4d641d61751aaf9a9422faf88b14b727789a0319fe872ee418",
				"Secretexampletestdata":          "355c838b5878c899babc73dbde367b0f450f37c41e5fec9c4af0a86900086b72",
				"SecretexampletestotherData":     "3f564266de9477b004c53c67de5eb4ec7cedb6dcee5b3d6d77ca2ed6cdd323ca",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := NewDependenciesMutator(test.objects)
			dm, ok := m.(*dependenciesMutator)
			require.True(t, ok)
			assert.Equal(t, test.expectedMap, dm.checksumsMap)
		})
	}
}

func TestDependenciesMutatorCanHandleResource(t *testing.T) {
	t.Parallel()

	hashesMap := map[string]string{
		"key": "hash",
	}

	tests := map[string]struct {
		hashesMap      map[string]string
		obj            *metav1.PartialObjectMetadata
		expectedResult bool
	}{
		"empty hash return alway false": {
			hashesMap:      make(map[string]string),
			expectedResult: false,
		},
		"config map is not handled": {
			hashesMap: hashesMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       configMapGK.Kind,
					APIVersion: "v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: false,
		},
		"deployment return true": {
			hashesMap: hashesMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       deployGK.Kind,
					APIVersion: "apps/v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: true,
		},
		"daemonset return true": {
			hashesMap: hashesMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       dsGK.Kind,
					APIVersion: "apps/v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: true,
		},
		"stateful return true": {
			hashesMap: hashesMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       stsGK.Kind,
					APIVersion: "apps/v1alpha1", // version is ignored put impossible one
				},
			},
			expectedResult: true,
		},
		"pod return true": {
			hashesMap: hashesMap,
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
			dm := dependenciesMutator{checksumsMap: test.hashesMap}
			assert.Equal(t, test.expectedResult, dm.CanHandleResource(test.obj))
		})
	}
}

func TestDependenciesMutatorMutate(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "dependency-mutator")

	checksumsMap := map[string]string{
		"ConfigMapexampleother-ns":       "89557268706287308eefe82fbdbd327914ac22a23499a45be72ea79483d73a73",
		"ConfigMapexampleother-nsconfig": "efe6c20174d12ac077f3de985cae67175a6108ac084e813a7d0eb9c5ff9fe85d",
		"ConfigMapexampletest":           "474402695ca63dd67a8ee93690d46011d2e19181aeb10c616af3cb48ac36adad",
		"ConfigMapexampletestbconfig":    "f2ad8ef38c3f6fac4d0dcfa67696710c04bc88f92a54f6758cb43c0d392b3eea",
		"ConfigMapexampletestconfig":     "3f564266de9477b004c53c67de5eb4ec7cedb6dcee5b3d6d77ca2ed6cdd323ca",
		"Secretexampletest":              "f8ac3c753041ab4d641d61751aaf9a9422faf88b14b727789a0319fe872ee418",
		"Secretexampletestdata":          "355c838b5878c899babc73dbde367b0f450f37c41e5fec9c4af0a86900086b72",
		"SecretexampletestotherData":     "3f564266de9477b004c53c67de5eb4ec7cedb6dcee5b3d6d77ca2ed6cdd323ca",
	}
	tests := map[string]struct {
		resource       *unstructured.Unstructured
		expectedResult *unstructured.Unstructured
		expectedError  string
	}{
		"deployment": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-deployment.yaml")),
		},
		"sts": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "sts.yaml")),
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-sts.yaml")),
		},
		"daemonset": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "daemonset.yaml")),
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-daemonset.yaml")),
		},
		"pod": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "pod.yaml")),
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-pod.yaml")),
		},
		"wrong resource": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-resource.yaml")),
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "wrong-resource.yaml")),
			expectedError:  `unsupported object type for dependencies mutator: "v1, Service"`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			mutator := &dependenciesMutator{
				checksumsMap: checksumsMap,
			}

			err := mutator.Mutate(test.resource, nil)
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
