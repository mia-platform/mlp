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

package resourceutil

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const testdata = "testdata/"

func TestNewResources(t *testing.T) {
	t.Run("Read a valid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "kubernetesersource.yaml")
		actual, err := NewResources(filePath, "default")
		require.Nil(t, err)
		expected := map[string]interface{}{"apiVersion": "v1", "data": map[string]interface{}{"dueKey": "deuValue", "unaKey": "unValue"}, "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "literal", "namespace": "default"}}
		require.Nil(t, err, "Reading a valid k8s file err must be nil")
		require.Equal(t, len(actual), 1, "1 Resource")
		require.Equal(t, actual[0].GroupVersionKind.Kind, "ConfigMap")
		require.EqualValues(t, expected, actual[0].Object.Object, "confimap on disk different")
	})
	t.Run("Read 2 valid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "tworesources.yaml")
		actual, err := NewResources(filePath, "default")
		expected1 := map[string]interface{}{"apiVersion": "v1", "data": map[string]interface{}{"dueKey": "deuValue", "unaKey": "unValue"}, "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "literal", "namespace": "default"}}
		expected2 := map[string]interface{}{"apiVersion": "v1", "data": map[string]interface{}{"dueKey": "deuValue2", "unaKey": "unValue2"}, "kind": "ConfigMap", "metadata": map[string]interface{}{"name": "literal2", "namespace": "default"}}
		require.Nil(t, err, "Reading two valid k8s file err must be nil")
		require.Equal(t, len(actual), 2, "2 Resource")
		require.Equal(t, actual[0].GroupVersionKind.Kind, "ConfigMap")
		require.Equal(t, actual[1].GroupVersionKind.Kind, "ConfigMap")
		require.EqualValues(t, expected1, actual[0].Object.Object, "confimap 1 on disk different")
		require.EqualValues(t, expected2, actual[1].Object.Object, "confimap 2 on disk different")
	})
	t.Run("Read not standard resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "non-standard-resource.yaml")
		actual, err := NewResources(filePath, "default")
		expected := map[string]interface{}{"apiVersion": "traefik.containo.us/v1alpha1", "kind": "IngressRoute", "metadata": map[string]interface{}{"name": "ingressroute1", "namespace": "default"}, "spec": map[string]interface{}{"entryPoints": []interface{}{"websecure"}, "routes": []interface{}{}}}
		require.Nil(t, err, "Reading non standard k8s file err must be nil")
		require.Equal(t, len(actual), 1, "1 Resource")
		require.EqualValues(t, expected, actual[0].Object.Object, "even a crd is unstructurable")
	})
	t.Run("Read an invalid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "invalidresource.yaml")
		_, err := NewResources(filePath, "default")
		require.EqualError(t, err, "resource testdata/invalidresource.yaml: error converting YAML to JSON: yaml: line 3: could not find expected ':'")
	})
}

func TestMakeResources(t *testing.T) {
	testCases := []struct {
		desc       string
		inputFiles []string
		expected   int
	}{
		{
			desc:       "3 valid resources in 2 files",
			inputFiles: []string{"kubernetesersource.yaml", "tworesources.yaml"},
			expected:   3,
		},
		{
			desc:       "resource with ---",
			inputFiles: []string{"configmap-with-minus.yaml"},
			expected:   1,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			var filePath []string
			for _, v := range tC.inputFiles {
				filePath = append(filePath, filepath.Join(testdata, v))
			}
			actual, err := MakeResources(filePath, "default")
			require.Nil(t, err)
			require.Equal(t, tC.expected, len(actual))
		})
	}
}

func TestGetKeysFromMap(t *testing.T) {
	testcases := []struct {
		description string
		input       map[string]bool
		expected    []string
	}{
		{
			description: "With duplicate",
			input:       map[string]bool{"a": false, "b": false, "c": true},
			expected:    []string{"a", "b", "c"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			res := getKeysFromMap(tt.input)

			require.Subset(t, tt.expected, res)
		})
	}
}

func TestGetMiaAnnotation(t *testing.T) {
	testcases := []struct {
		description string
		input       string
		expected    string
	}{
		{
			description: "Using a simple name",
			input:       "name",
			expected:    "mia-platform.eu/name",
		},
		{
			description: "Using space between name",
			input:       "na me",
			expected:    "mia-platform.eu/na-me",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			res := GetMiaAnnotation(tt.input)

			require.Equal(t, tt.expected, res)
		})
	}
}
func TestGetChecksum(t *testing.T) {
	testcases := []struct {
		description string
		input       []byte
		expected    string
	}{
		{
			description: "Correctly calculate checksum from bytes as input",
			input:       []byte("convert me in bytes"),
			expected:    "1a61c4caa88712cef548ed807e55822e7ae20fcd9f9d4f0ae135c064f20a7ebd",
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			res := GetChecksum(tt.input)

			require.Equal(t, tt.expected, res)
		})
	}
}

func TestMapSecretAndConfigMap(t *testing.T) {
	filePath := []string{
		filepath.Join(testdata, "kubernetesersource.yaml"),
		filepath.Join(testdata, "tworesources.yaml"),
		filepath.Join(testdata, "secretresource.yaml"),
	}

	resources, err := MakeResources(filePath, "default")
	require.Nil(t, err)

	map1, map2, err := MapSecretAndConfigMap(resources)
	require.Nil(t, err)
	require.Equal(t, map1, map[string]string{
		"literal":         "d9f8c57ed416d3d8775c0850f0122f7bc777864c231b0416d86509c0dfdfea9c",
		"literal-dueKey":  "89b0beacc9f0db0c9d3a535703ceb257a028c45097b572e11383fb04521766a7",
		"literal-unaKey":  "c2525d273c01748e32af2cedba700f68c45031b12de45a6cdc8f4cb7cf21f72b",
		"literal2":        "3f1f8218f370ac0726cb97fb82c0b5ec302b84be2fc8ac6db52286c361f4af4b",
		"literal2-dueKey": "3982db497aefc74d6fb97a2a4b8dd729073c82735a9eafc0bbe5485b2fe653f8",
		"literal2-unaKey": "da8a5ae64a6172ad008d793f0885cb003d3d5eeaf349bcea39871aa78e3ab982"},
		"configmap map")
	require.Equal(t, map2, map[string]string{
		"opaque":      "7522c25ea135fa86f47443a90b2c154a5c6266b3b83c3104e43d898a9ad11881",
		"opaque-key1": "232e1000c4504e432a09d6d74d5bba9a27533354c9872fdcd1934b42876c0faa",
	}, "secret map")
}

func TestGetPodsDependencies(t *testing.T) {
	secretVolume := corev1.Volume{
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: "secret",
			},
		},
	}

	secretVolume2 := corev1.Volume{
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: "secret2",
			},
		},
	}

	configMapVolume := corev1.Volume{
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "configMap",
				},
			},
		},
	}

	configMapVolume2 := corev1.Volume{
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "configMap2",
				},
			},
		},
	}

	containerWithEnv := corev1.Container{
		Env: []corev1.EnvVar{
			{
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "env-config-map",
						},
					},
				},
			},
			{
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "env-secret",
						},
					},
				},
			},
		},
	}

	containerWithRedundantName := corev1.Container{
		Env: []corev1.EnvVar{
			{
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "configMap",
						},
					},
				},
			},
			{
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "secret",
						},
					},
				},
			},
		},
	}

	containerWithKeys := corev1.Container{
		Env: []corev1.EnvVar{
			{
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "configMapWithKey",
						},
						Key: "configMapKey",
					},
				},
			},
			{
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "secretWithKey",
						},
						Key: "secretKey",
					},
				},
			},
		},
	}

	containerWithKeysButVolumeConflicts := corev1.Container{
		Env: []corev1.EnvVar{
			{
				ValueFrom: &corev1.EnvVarSource{
					ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "configMap",
						},
						Key: "configMapKey",
					},
				},
			},
			{
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "secret",
						},
						Key: "secretKey",
					},
				},
			},
		},
	}

	testcases := []struct {
		description string
		input       corev1.PodSpec
		expected    map[string][]string
	}{
		{
			description: "with Volume one secret",
			input: corev1.PodSpec{
				Volumes: []corev1.Volume{
					secretVolume,
				},
			},
			expected: map[string][]string{
				"ConfigMap": {},
				"Secret":    {"secret"},
			},
		},
		{
			description: "with Volume one configmap",
			input: corev1.PodSpec{
				Volumes: []corev1.Volume{
					configMapVolume,
				},
			},
			expected: map[string][]string{
				"ConfigMap": {"configMap"},
				"Secret":    {},
			},
		},
		{
			description: "with Volume one configmap and one secret",
			input: corev1.PodSpec{
				Volumes: []corev1.Volume{
					configMapVolume,
					secretVolume,
				},
			},
			expected: map[string][]string{
				"ConfigMap": {"configMap"},
				"Secret":    {"secret"},
			},
		},
		{
			description: "with Volume two configmaps and two secrets",
			input: corev1.PodSpec{
				Volumes: []corev1.Volume{
					configMapVolume,
					configMapVolume2,
					secretVolume,
					secretVolume2,
				},
			},
			expected: map[string][]string{
				"ConfigMap": {"configMap", "configMap2"},
				"Secret":    {"secret", "secret2"},
			},
		},
		{
			description: "with Containers one secret and one configmap",
			input: corev1.PodSpec{
				Containers: []corev1.Container{
					containerWithEnv,
				},
			},
			expected: map[string][]string{
				"ConfigMap": {"env-config-map"},
				"Secret":    {"env-secret"},
			},
		},
		{
			description: "with Containers and Volumes",
			input: corev1.PodSpec{
				Containers: []corev1.Container{
					containerWithEnv,
					containerWithRedundantName,
				},
				Volumes: []corev1.Volume{
					configMapVolume,
					configMapVolume2,
					secretVolume,
					secretVolume2,
				},
			},
			expected: map[string][]string{
				"ConfigMap": {"configMap", "configMap2", "env-config-map"},
				"Secret":    {"secret", "secret2", "env-secret"},
			},
		},
		{
			description: "with Containers having keys",
			input: corev1.PodSpec{
				Containers: []corev1.Container{
					containerWithEnv,
					containerWithKeys,
				},
			},
			expected: map[string][]string{
				"ConfigMap": {"env-config-map", "configMapWithKey-configMapKey"},
				"Secret":    {"env-secret", "secretWithKey-secretKey"},
			},
		},
		{
			description: "with Containers having keys but volume already mount all",
			input: corev1.PodSpec{
				Containers: []corev1.Container{
					containerWithKeysButVolumeConflicts,
				},
				Volumes: []corev1.Volume{
					configMapVolume,
					secretVolume,
				},
			},
			expected: map[string][]string{
				"ConfigMap": {"configMap"},
				"Secret":    {"secret"},
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			res := GetPodsDependencies(tt.input)

			require.Subset(t, tt.expected["ConfigMap"], res["ConfigMap"])
			require.Subset(t, tt.expected["Secret"], res["Secret"])
		})
	}
}

func TestIsNotUsingSemver(t *testing.T) {
	testcases := []struct {
		description string
		input       []interface{}
		expected    bool
	}{
		{

			description: "following semver",
			input:       []interface{}{map[string]interface{}{"image": "test:1.0.0"}},
			expected:    false,
		},
		{
			description: "not following semver",
			input:       []interface{}{map[string]interface{}{"image": "test:latest"}},
			expected:    true,
		},
		{
			description: "all following semver",
			input: []interface{}{map[string]interface{}{"image": "test:1.0.0"},
				map[string]interface{}{"image": "test:1.0.0-alpha"},
				map[string]interface{}{"image": "test:1.0.0+20130313144700"},
				map[string]interface{}{"image": "test:1.0.0-beta+exp.sha.5114f85"}},
			expected: false,
		},
		{
			description: "one not following semver",
			input: []interface{}{map[string]interface{}{"image": "test:1.0.0"},
				map[string]interface{}{"image": "test:1.0.0-alpha"},
				map[string]interface{}{"image": "test:1.0.0+20130313144700"},
				map[string]interface{}{"image": "test:tag1"},
			},
			expected: true,
		},
	}

	for _, tt := range testcases {
		types := []struct {
			typ            string
			path           string
			containersPath []string
		}{
			{
				typ:            "deployments",
				path:           "../deploy/testdata/test-deployment.yaml",
				containersPath: []string{"spec", "template", "spec", "containers"},
			},
			{
				typ:            "cronjobs",
				path:           "../deploy/testdata/cronjob-test.cronjob.yml",
				containersPath: []string{"spec", "jobTemplate", "spec", "template", "spec", "containers"},
			},
		}
		for _, typ := range types {
			t.Run(fmt.Sprintf("%s - %s", typ.typ, tt.description), func(t *testing.T) {
				targetObject, err := NewResources(typ.path, "default")
				require.Nil(t, err)
				err = unstructured.SetNestedField(targetObject[0].Object.Object, tt.input, typ.containersPath...)
				require.Nil(t, err)
				boolRes, err := IsNotUsingSemver(&targetObject[0])
				require.Nil(t, err)
				require.Equal(t, tt.expected, boolRes)
			})
		}
	}
}
