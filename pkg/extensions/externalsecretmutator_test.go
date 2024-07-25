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
	"path/filepath"
	"testing"

	extsecv1beta1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestNewExternalSecretMutator(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "externalsecret-mutator")
	deployment := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml"))
	store := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "store.yaml"))
	extSec := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "external-secret.yaml"))
	extSec2 := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "external-secret-secret-name.yaml"))
	configmap := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "configmap.yaml"))

	tests := map[string]struct {
		objects                    []*unstructured.Unstructured
		expectedExternalSeceretMap map[string]*unstructured.Unstructured
		expectedSecretsStoreMap    map[string]*unstructured.Unstructured
	}{
		"empty objects": {
			expectedExternalSeceretMap: map[string]*unstructured.Unstructured{},
			expectedSecretsStoreMap:    map[string]*unstructured.Unstructured{},
		},
		"no external secrets or stores in objects": {
			objects: []*unstructured.Unstructured{
				deployment,
			},
			expectedExternalSeceretMap: map[string]*unstructured.Unstructured{},
			expectedSecretsStoreMap:    map[string]*unstructured.Unstructured{},
		},
		"mixed objects": {
			objects: []*unstructured.Unstructured{
				deployment,
				store,
				extSec,
				extSec2,
				configmap,
			},
			expectedExternalSeceretMap: map[string]*unstructured.Unstructured{
				"external-secret:externalsecret-test":    extSec,
				"custom-secret-name:externalsecret-test": extSec2,
			},
			expectedSecretsStoreMap: map[string]*unstructured.Unstructured{
				"SecretStore:secret-store:externalsecret-test": store,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := NewExternalSecretsMutator(test.objects)
			esm, ok := m.(*externalSecretsMutator)
			require.True(t, ok)
			assert.Equal(t, test.expectedExternalSeceretMap, esm.externalSecretMap)
			assert.Equal(t, test.expectedSecretsStoreMap, esm.secretsStores)
		})
	}
}

func TestExternalSecretMutatorCanHandleResource(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "externalsecret-mutator")
	externalSecretMap := map[string]*unstructured.Unstructured{
		"key": jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
	}

	tests := map[string]struct {
		externalSecretMap map[string]*unstructured.Unstructured
		obj               *metav1.PartialObjectMetadata
		expectedResult    bool
	}{
		"empty external secret map always returns false": {
			externalSecretMap: make(map[string]*unstructured.Unstructured),
			expectedResult:    false,
		},
		"config map is not handled": {
			externalSecretMap: externalSecretMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       configMapGK.Kind,
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
			},
			expectedResult: false,
		},
		"deployment return true": {
			externalSecretMap: externalSecretMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       deployGK.Kind,
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
			},
			expectedResult: true,
		},
		"daemonset return true": {
			externalSecretMap: externalSecretMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       dsGK.Kind,
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
			},
			expectedResult: true,
		},
		"stateful return true": {
			externalSecretMap: externalSecretMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       stsGK.Kind,
					APIVersion: appsv1.SchemeGroupVersion.String(),
				},
			},
			expectedResult: true,
		},
		"pod return true": {
			externalSecretMap: externalSecretMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       podGK.Kind,
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
			},
			expectedResult: true,
		},
		"external secret return true": {
			externalSecretMap: externalSecretMap,
			obj: &metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       extsecGK.Kind,
					APIVersion: extsecv1beta1.SchemeGroupVersion.String(),
				},
			},
			expectedResult: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			esm := externalSecretsMutator{externalSecretMap: test.externalSecretMap}
			assert.Equal(t, test.expectedResult, esm.CanHandleResource(test.obj))
		})
	}
}

func TestExternalSecretMutatorMutate(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "externalsecret-mutator")
	externalSecretsMap := map[string]*unstructured.Unstructured{
		"external-secret:externalsecret-test":    jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "external-secret.yaml")),
		"custom-secret-name:externalsecret-test": jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "external-secret-secret-name.yaml")),
	}
	secretsStores := map[string]*unstructured.Unstructured{
		"SecretStore:secret-store:externalsecret-test": jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "store.yaml")),
	}

	tests := map[string]struct {
		resource       *unstructured.Unstructured
		expectedResult *unstructured.Unstructured
		expectedError  string
	}{
		"external-secret": {
			resource:       jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "external-secret.yaml")),
			expectedResult: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "expected-external-secret.yaml")),
		},
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
			mutator := &externalSecretsMutator{
				externalSecretMap: externalSecretsMap,
				secretsStores:     secretsStores,
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
