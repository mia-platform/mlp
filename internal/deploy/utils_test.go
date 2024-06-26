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

// import (
// 	"reflect"
// 	"testing"

// 	"github.com/mia-platform/mlp/pkg/resourceutil"
// 	"github.com/stretchr/testify/require"

// 	corev1 "k8s.io/api/core/v1"
// 	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
// 	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
// 	"k8s.io/apimachinery/pkg/runtime"
// 	"k8s.io/apimachinery/pkg/runtime/schema"
// 	"k8s.io/client-go/dynamic/fake"
// )

// func TestMakeResourceMap(t *testing.T) {
// 	gvkSecret := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
// 	gvkCm := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}

// 	testcases := []struct {
// 		description string
// 		input       []resourceutil.Resource
// 		expected    map[string]*ResourceList
// 	}{
// 		{
// 			description: "All secrets",
// 			input: []resourceutil.Resource{
// 				{
// 					Object:           unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "metadata": map[string]interface{}{"name": "secret1"}}},
// 					GroupVersionKind: &gvkSecret,
// 				},
// 				{
// 					Object:           unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "metadata": map[string]interface{}{"name": "secret2"}}},
// 					GroupVersionKind: &gvkSecret,
// 				},
// 			},
// 			expected: map[string]*ResourceList{"Secret": {
// 				Gvk:       &gvkSecret,
// 				Resources: []string{"secret1", "secret2"},
// 			}},
// 		},
// 		{
// 			description: "1 secret 1 cm",
// 			input: []resourceutil.Resource{
// 				{
// 					Object:           unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "metadata": map[string]interface{}{"name": "secret1"}}},
// 					GroupVersionKind: &gvkSecret,
// 				},
// 				{
// 					Object:           unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "metadata": map[string]interface{}{"name": "cm1"}}},
// 					GroupVersionKind: &gvkCm,
// 				},
// 			},
// 			expected: map[string]*ResourceList{"Secret": {
// 				Gvk:       &gvkSecret,
// 				Resources: []string{"secret1"},
// 			},
// 				"ConfigMap": {
// 					Gvk:       &gvkCm,
// 					Resources: []string{"cm1"},
// 				},
// 			},
// 		},
// 	}

// 	for _, tt := range testcases {
// 		t.Run(tt.description, func(t *testing.T) {
// 			actual := makeResourceMap(tt.input)
// 			require.Equal(t, tt.expected, actual)
// 		})
// 	}
// }

// func TestGetOldResourceMap(t *testing.T) {
// 	secretGvk := &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}
// 	configMapGvk := &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}
// 	deploymentGvk := &schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
// 	certGvk := &schema.GroupVersionKind{Group: "cert-manager.io", Version: "v1", Kind: "Certificate"}
// 	mappingGvk := &schema.GroupVersionKind{Group: "getambassador.io", Version: "v2", Kind: "Mapping"}

// 	testcases := []struct {
// 		description string
// 		input       *corev1.Secret
// 		expected    map[string]*ResourceList
// 		error       func(t *testing.T, err error)
// 	}{
// 		{
// 			description: "resources field unmarshal is correct",
// 			input: &corev1.Secret{
// 				Data: map[string][]byte{"resources": []byte(`{
// 					"Secret":{"kind":{"Group": "", "Version": "v1", "Kind": "Secret"}, "resources":["foo", "bar"]},
// 					"ConfigMap": {"kind":{"Group": "", "Version": "v1", "Kind": "ConfigMap"}, "resources":[]},
// 					"Deployment":{"kind":{"Group":"apps","Version":"v1","Kind":"Deployment"}, "resources":["foo"]},
// 					"Certificate":{"kind":{"Group":"cert-manager.io","Version":"v1","Kind":"Certificate"},"resources":["my-cert"]}
// 				}`)},
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      resourceSecretName,
// 					Namespace: "foo",
// 				},
// 				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
// 			},
// 			expected: map[string]*ResourceList{
// 				"Secret":      {Gvk: secretGvk, Resources: []string{"foo", "bar"}},
// 				"ConfigMap":   {Gvk: configMapGvk, Resources: []string{}},
// 				"Deployment":  {Gvk: deploymentGvk, Resources: []string{"foo"}},
// 				"Certificate": {Gvk: certGvk, Resources: []string{"my-cert"}},
// 			},
// 			error: func(t *testing.T, err error) {
// 				t.Helper()
// 				require.Nil(t, err)
// 			},
// 		},
// 		{
// 			description: "with a Mapping kind",
// 			input: &corev1.Secret{
// 				Data: map[string][]byte{"resources": []byte(`{"Mapping":{"kind":{"Group":"getambassador.io","Version":"v2","Kind":"Mapping"},"resources":["a-resource","another-resource"]}}`)},
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      resourceSecretName,
// 					Namespace: "foo",
// 				},
// 				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
// 			},
// 			expected: map[string]*ResourceList{
// 				"Mapping": {Gvk: mappingGvk, Resources: []string{"a-resource", "another-resource"}},
// 			},
// 			error: func(t *testing.T, err error) {
// 				t.Helper()
// 				require.Nil(t, err)
// 			},
// 		},
// 		{
// 			description: "resources in in v0 format",
// 			input: &corev1.Secret{
// 				Data: map[string][]byte{"resources": []byte(`{"Deployment":{"kind":"Deployment","Mapping":{"Group":"apps","Version":"v1","Resource":"deployments"},"resources":["test-deployment","test-deployment-2"]}}`)},
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      resourceSecretName,
// 					Namespace: "foo",
// 				},
// 				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
// 			},
// 			expected: map[string]*ResourceList{"Deployment": {Resources: []string{"test-deployment", "test-deployment-2"}, Gvk: &schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}}},
// 			error: func(t *testing.T, err error) {
// 				t.Helper()
// 				require.Nil(t, err)
// 			},
// 		},
// 		{
// 			description: "resource field is empty",
// 			input: &corev1.Secret{
// 				Data: map[string][]byte{"resources": []byte(`{}`)},
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      resourceSecretName,
// 					Namespace: "foo",
// 				},
// 				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
// 			},
// 			expected: map[string]*ResourceList{},
// 			error: func(t *testing.T, err error) {
// 				t.Helper()
// 				require.Nil(t, err)
// 			},
// 		},
// 		{
// 			description: "resource field does not contain map[string][]string but map[string]string ",
// 			input: &corev1.Secret{
// 				Data: map[string][]byte{"resources": []byte(`{ "foo": "bar" `)},
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      resourceSecretName,
// 					Namespace: "foo",
// 				},
// 				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
// 			},
// 			expected: nil,
// 			error: func(t *testing.T, err error) {
// 				t.Helper()
// 				require.NotNil(t, err)
// 			},
// 		},
// 		{
// 			description: "secret is not found",
// 			input: &corev1.Secret{
// 				Data: map[string][]byte{"resources": []byte(`{ "foo": "bar" `)},
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      "wrong-name",
// 					Namespace: "foo",
// 				},
// 				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
// 			},
// 			expected: map[string]*ResourceList{},
// 			error: func(t *testing.T, err error) {
// 				t.Helper()
// 				require.Nil(t, err)
// 			},
// 		},
// 	}

// 	scheme := runtime.NewScheme()
// 	err := corev1.AddToScheme(scheme)
// 	require.Nil(t, err)
// 	for _, tt := range testcases {
// 		t.Run(tt.description, func(t *testing.T) {
// 			dynamicClient := fake.NewSimpleDynamicClient(scheme, tt.input)
// 			actual, err := getOldResourceMap(&k8sClients{dynamic: dynamicClient}, "foo")
// 			tt.error(t, err)
// 			require.Equal(t, tt.expected, actual)
// 		})
// 	}
// }

// func TestDeletedResources(t *testing.T) {
// 	testcases := []struct {
// 		description string
// 		old         map[string]*ResourceList
// 		new         map[string]*ResourceList
// 		expected    map[string]*ResourceList
// 	}{
// 		{
// 			description: "No diff with equal maps",
// 			old:         map[string]*ResourceList{"secrets": {Resources: []string{"foo", "bar"}}},
// 			new:         map[string]*ResourceList{"secrets": {Resources: []string{"foo", "bar"}}},
// 			expected:    map[string]*ResourceList{},
// 		},
// 		{
// 			description: "Expected old map if new is empty",
// 			old:         map[string]*ResourceList{"secrets": {Resources: []string{"foo", "bar"}}},
// 			new:         map[string]*ResourceList{"secrets": {Resources: []string{}}},
// 			expected:    map[string]*ResourceList{"secrets": {Resources: []string{"foo", "bar"}}},
// 		},
// 		{
// 			description: "Remove one resource from resourceList",
// 			old:         map[string]*ResourceList{"secrets": {Resources: []string{"foo", "bar"}}},
// 			new:         map[string]*ResourceList{"secrets": {Resources: []string{"foo"}}},
// 			expected:    map[string]*ResourceList{"secrets": {Resources: []string{"bar"}}},
// 		},
// 		{
// 			description: "Add one resource type",
// 			old:         map[string]*ResourceList{"secrets": {Resources: []string{"foo", "bar"}}},
// 			new:         map[string]*ResourceList{"secrets": {Resources: []string{"foo"}}, "configmaps": {Resources: []string{"foo"}}},
// 			expected:    map[string]*ResourceList{"secrets": {Resources: []string{"bar"}}},
// 		},
// 		{
// 			description: "Delete one resource type",
// 			old:         map[string]*ResourceList{"secrets": {Resources: []string{"foo"}}, "configmaps": {Resources: []string{"foo"}}},
// 			new:         map[string]*ResourceList{"secrets": {Resources: []string{"foo"}}},
// 			expected:    map[string]*ResourceList{"configmaps": {Resources: []string{"foo"}}},
// 		},
// 	}

// 	for _, tt := range testcases {
// 		t.Run(tt.description, func(t *testing.T) {
// 			actual := deletedResources(tt.new, tt.old)
// 			require.True(t, reflect.DeepEqual(tt.expected, actual))
// 		})
// 	}
// }

// func TestDiffResourceArray(t *testing.T) {
// 	testcases := []struct {
// 		description string
// 		old         []string
// 		new         []string
// 		expected    []string
// 	}{
// 		{
// 			description: "No diff with equal slices",
// 			old:         []string{"foo", "bar"},
// 			new:         []string{"foo", "bar"},
// 			expected:    []string{},
// 		},
// 		{
// 			description: "Expected old array if new is empty",
// 			old:         []string{"foo", "bar"},
// 			new:         []string{},
// 			expected:    []string{"foo", "bar"},
// 		},
// 	}

// 	for _, tt := range testcases {
// 		t.Run(tt.description, func(t *testing.T) {
// 			actual := diffResourceArray(tt.new, tt.old)
// 			require.Equal(t, tt.expected, actual)
// 		})
// 	}
// }

// func TestContains(t *testing.T) {
// 	testcases := []struct {
// 		description string
// 		array       []string
// 		element     string
// 		expected    bool
// 	}{
// 		{
// 			description: "the element is contained in the slice",
// 			array:       []string{"foo", "bar"},
// 			element:     "foo",
// 			expected:    true,
// 		},
// 		{
// 			description: "the element is not contained in the slice",
// 			array:       []string{"foo", "bar"},
// 			element:     "foobar",
// 			expected:    false,
// 		},
// 		{
// 			description: "the element is not contained in empty slice",
// 			array:       []string{},
// 			element:     "foobar",
// 			expected:    false,
// 		},
// 	}

// 	for _, tt := range testcases {
// 		t.Run(tt.description, func(t *testing.T) {
// 			actual := contains(tt.array, tt.element)
// 			require.Equal(t, tt.expected, actual)
// 		})
// 	}
// }

// func TestConvertSecretFormat(t *testing.T) {
// 	oldres := []byte("{\"Deployment\":{\"kind\":\"Deployment\",\"Mapping\":{\"Group\":\"apps\",\"Version\":\"v1\",\"Resource\":\"deployments\"},\"resources\":[\"test-deployment\",\"test-deployment-2\"]}}")
// 	actual, err := convertSecretFormat(oldres)
// 	require.Nil(t, err)
// 	require.Equal(t, []string{"test-deployment", "test-deployment-2"}, actual["Deployment"].Resources)
// 	require.Equal(t, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, *actual["Deployment"].Gvk)
// }
