package resourceutil

import (
	"path/filepath"
	"testing"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/resource"
)

func TestNewResource(t *testing.T) {
	t.Run("Read a valid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "kubernetesersource.yaml")

		actual, err := NewResource(filePath)
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
				GroupVersion: "v1",
				Kind:         "ConfigMap",
				Metadata:     &expectedMetadata,
			},
		}
		require.Nil(t, err, "Reading a valid k8s file err must be nil")
		require.Equal(t, actual, expected, "Resource read from file must be equal to expected")
	})

	t.Run("Read an invalid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "notarresource.yaml")

		resource, err := NewResource(filePath)
		require.Nil(t, resource, "Reading an invalid k8s file resource must be nil")
		require.NotNil(t, err, "Reading an invalid k8s file resource an error must be returned")
	})
}

func TestMakeInfo(t *testing.T) {

	cf := &apiv1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "literal",
		},
		Data: map[string]string{
			"dueKey": "deuValue",
			"unaKey": "unValue",
		},
	}

	t.Run("File with two resources", func(t *testing.T) {
		b := NewFakeBuilder()
		_, err := MakeInfo(b, "default", "testdata/tworesources.yaml")

		require.EqualError(t, err, "Multiple objects in single yaml file currently not supported")
	})

	t.Run("resource built with correct namespace", func(t *testing.T) {
		b := NewFakeBuilder()
		b.AddResources([]runtime.Object{cf}, false)
		info, err := MakeInfo(b, "default", "testdata/kubernetesersource.yaml")
		require.Nil(t, err)
		require.Equal(t, "default", info.Namespace, "The resource namespace must be the one passed as parameter")
	})
}

func TestMergeLabels(t *testing.T) {

	testcases := []struct {
		description string
		message     string
		expected    map[string]string
		current     map[string]string
		changes     map[string]string
	}{
		{
			description: "Update value in map",
			message:     "The value should be updated with the one contained in changes map",
			expected: map[string]string{
				"foo": "foo",
				"bar": "bar",
			},

			current: map[string]string{
				"foo": "foo",
				"bar": "foo",
			},

			changes: map[string]string{
				"bar": "bar",
			},
		},
		{
			description: "Add new key value in map",
			message:     "The new key value should be present in the new map",
			expected: map[string]string{
				"foo":    "foo",
				"bar":    "bar",
				"foobar": "foo",
			},

			current: map[string]string{
				"foo": "foo",
				"bar": "bar",
			},

			changes: map[string]string{
				"foobar": "foo",
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			actual := mergeLabels(tt.current, tt.changes)
			require.Equal(t, tt.expected, actual, tt.message)
		})
	}
}

func TestUpdateLabels(t *testing.T) {

	testcases := []struct {
		description string
		message     string
		expected    map[string]string
		current     runtime.Object
		changes     map[string]string
	}{
		{
			description: "Add label to an object",
			message:     "The updated object labels should contain the new key value",
			expected: map[string]string{
				"foo": "foo",
				"bar": "bar",
			},

			current: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "Deployment"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
			},

			changes: map[string]string{
				ManagedByLabel: ManagedByMia,
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			err := updateLabels(tt.current, tt.changes)
			require.Nil(t, err)
			labels, err := accessor.Labels(tt.current)
			require.Nil(t, err)
			require.Equal(t, labels[ManagedByLabel], ManagedByMia, tt.message)
		})
	}
}

func TestMakeResource(t *testing.T) {
	filePath := filepath.Join(testdata, "kubernetesersource.yaml")

	cf := &apiv1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "literal",
		},
		Data: map[string]string{
			"dueKey": "deuValue",
			"unaKey": "unValue",
		},
	}

	builder := NewFakeBuilder()
	builder.AddResources([]runtime.Object{cf}, false)

	head := ResourceHead{
		GroupVersion: cf.APIVersion,
		Kind:         cf.Kind,
	}

	resource, err := MakeResource(builder, "bar", filePath)
	require.Nil(t, err)
	require.Equal(t, filePath, resource.Filepath, "the paths should coincide")
	require.Equal(t, "bar", resource.Namespace, "the namespaces should coincide")
	require.Equal(t, cf.Name, resource.Name, "the names should coincide")
	require.Equal(t, head.Kind, resource.Head.Kind, "the kinds should coincide")
	require.Equal(t, head.GroupVersion, resource.Head.GroupVersion, "the groupversions should coincide")

	objMeta, err := meta.Accessor(resource.Info.Object)
	require.Nil(t, err)

	require.Equal(t, ManagedByMia, objMeta.GetLabels()[ManagedByLabel], "should contain the managed by MIA label")
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
			description: "Correclty calculate checksum from bytes as input",
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

	infoDeployment := &resource.Info{
		Object: &appsv1.Deployment{
			TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{},
			Spec: appsv1.DeploymentSpec{
				Template: v1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{},
				},
			},
		},
	}

	infoConfigMap := &resource.Info{
		Object: &apiv1.ConfigMap{
			TypeMeta: metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			Data:     map[string]string{"name": "name1", "time": "2"},
		},
	}

	infoBinaryConfigMap := &resource.Info{
		Object: &apiv1.ConfigMap{
			TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
			BinaryData: map[string][]byte{"binName": []byte("name")},
		},
	}

	infoSecret := &resource.Info{
		Object: &apiv1.Secret{
			TypeMeta:   metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			StringData: map[string]string{"name": "secret"},
		},
	}

	infoBinarySecret := &resource.Info{
		Object: &apiv1.Secret{
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			Data:     map[string][]byte{"binSecret": []byte("name")},
		},
	}

	testcases := []struct {
		description          string
		input                []Resource
		expectedConfigMapMap map[string]string
		expectedSecretMap    map[string]string
	}{
		{
			description: "Single configMap",
			input: []Resource{
				{
					Name: "configmap-name",
					Head: ResourceHead{Kind: "ConfigMap"},
					Info: infoConfigMap,
				},
			},
			expectedConfigMapMap: map[string]string{
				"configmap-name":      "6168737a06d058976783116253e72c93cf9c37735664cc9ab91d1939ad4a5c65",
				"configmap-name-name": "9367417d63903350aeb7e092bca792263d4fd82d4912252e014e073a8931b4c1",
				"configmap-name-time": "d4735e3a265e16eee03f59718b9b5d03019c07d8b6c51f90da3a666eec13ab35",
			},
			expectedSecretMap: map[string]string{},
		},
		{
			description: "Single binary configMap",
			input: []Resource{
				{
					Name: "bin-configmap-name",
					Head: ResourceHead{Kind: "ConfigMap"},
					Info: infoBinaryConfigMap,
				},
			},
			expectedConfigMapMap: map[string]string{
				"bin-configmap-name":         "cb7b618f5015758718976fff957ffca0513fd898a809c65ce4db3a8a325d6335",
				"bin-configmap-name-binName": "82a3537ff0dbce7eec35d69edc3a189ee6f17d82f353a553f9aa96cb0be3ce89",
			},
			expectedSecretMap: map[string]string{},
		},
		{
			description: "Single secret",
			input: []Resource{
				{
					Name: "secret-name",
					Head: ResourceHead{Kind: "Secret"},
					Info: infoSecret,
				},
			},
			expectedConfigMapMap: map[string]string{},
			expectedSecretMap: map[string]string{
				"secret-name":      "4622a7d32da26554ec12fa8cbf0fabe6a8f7ae67d0a8a99f50c1110137ab6e5e",
				"secret-name-name": "2bb80d537b1da3e38bd30361aa855686bde0eacd7162fef6a25fe97bf527a25b",
			},
		},
		{
			description: "Single binary secret",
			input: []Resource{
				{
					Name: "bin-secret-name",
					Head: ResourceHead{Kind: "Secret"},
					Info: infoBinarySecret,
				},
			},
			expectedConfigMapMap: map[string]string{},
			expectedSecretMap: map[string]string{
				"bin-secret-name":           "243cad3263de13111fb003737ab3caf8fc8162ec671eb34f82949a381af1e693",
				"bin-secret-name-binSecret": "82a3537ff0dbce7eec35d69edc3a189ee6f17d82f353a553f9aa96cb0be3ce89",
			},
		},
		{
			description: "Single deployment",
			input: []Resource{
				{
					Name: "deployment-name",
					Head: ResourceHead{Kind: "Deployment"},
					Info: infoDeployment,
				},
			},
			expectedConfigMapMap: map[string]string{},
			expectedSecretMap:    map[string]string{},
		},
		{
			description: "With some resources",
			input: []Resource{
				{
					Name: "deployment-name",
					Head: ResourceHead{Kind: "Deployment"},
					Info: infoDeployment,
				},
				{
					Name: "secret-name",
					Head: ResourceHead{Kind: "Secret"},
					Info: infoSecret,
				},
				{
					Name: "configmap-name",
					Head: ResourceHead{Kind: "ConfigMap"},
					Info: infoConfigMap,
				},
				{
					Name: "bin-secret-name",
					Head: ResourceHead{Kind: "Secret"},
					Info: infoBinarySecret,
				},
				{
					Name: "bin-configmap-name",
					Head: ResourceHead{Kind: "ConfigMap"},
					Info: infoBinaryConfigMap,
				},
			},
			expectedConfigMapMap: map[string]string{
				"configmap-name":             "6168737a06d058976783116253e72c93cf9c37735664cc9ab91d1939ad4a5c65",
				"configmap-name-name":        "9367417d63903350aeb7e092bca792263d4fd82d4912252e014e073a8931b4c1",
				"configmap-name-time":        "d4735e3a265e16eee03f59718b9b5d03019c07d8b6c51f90da3a666eec13ab35",
				"bin-configmap-name":         "cb7b618f5015758718976fff957ffca0513fd898a809c65ce4db3a8a325d6335",
				"bin-configmap-name-binName": "82a3537ff0dbce7eec35d69edc3a189ee6f17d82f353a553f9aa96cb0be3ce89",
			},
			expectedSecretMap: map[string]string{
				"secret-name":               "4622a7d32da26554ec12fa8cbf0fabe6a8f7ae67d0a8a99f50c1110137ab6e5e",
				"secret-name-name":          "2bb80d537b1da3e38bd30361aa855686bde0eacd7162fef6a25fe97bf527a25b",
				"bin-secret-name":           "243cad3263de13111fb003737ab3caf8fc8162ec671eb34f82949a381af1e693",
				"bin-secret-name-binSecret": "82a3537ff0dbce7eec35d69edc3a189ee6f17d82f353a553f9aa96cb0be3ce89",
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			cfmMap, secMap, err := MapSecretAndConfigMap(tt.input)
			require.Nil(t, err)
			require.Equal(t, tt.expectedConfigMapMap, cfmMap)
			require.Equal(t, tt.expectedSecretMap, secMap)
		})
	}
}

func TestGetPodsDependencies(t *testing.T) {

	secretVolume := apiv1.Volume{
		VolumeSource: apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: "secret",
			},
		},
	}

	secretVolume2 := apiv1.Volume{
		VolumeSource: apiv1.VolumeSource{
			Secret: &apiv1.SecretVolumeSource{
				SecretName: "secret2",
			},
		},
	}

	configMapVolume := apiv1.Volume{
		VolumeSource: apiv1.VolumeSource{
			ConfigMap: &apiv1.ConfigMapVolumeSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: "configMap",
				},
			},
		},
	}

	configMapVolume2 := apiv1.Volume{
		VolumeSource: apiv1.VolumeSource{
			ConfigMap: &apiv1.ConfigMapVolumeSource{
				LocalObjectReference: apiv1.LocalObjectReference{
					Name: "configMap2",
				},
			},
		},
	}

	containerWithEnv := apiv1.Container{
		Env: []apiv1.EnvVar{
			{
				ValueFrom: &apiv1.EnvVarSource{
					ConfigMapKeyRef: &apiv1.ConfigMapKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: "env-config-map",
						},
					},
				},
			},
			{
				ValueFrom: &apiv1.EnvVarSource{
					SecretKeyRef: &apiv1.SecretKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: "env-secret",
						},
					},
				},
			},
		},
	}

	containerWithRedundantName := apiv1.Container{
		Env: []apiv1.EnvVar{
			{
				ValueFrom: &apiv1.EnvVarSource{
					ConfigMapKeyRef: &apiv1.ConfigMapKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: "configMap",
						},
					},
				},
			},
			{
				ValueFrom: &apiv1.EnvVarSource{
					SecretKeyRef: &apiv1.SecretKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: "secret",
						},
					},
				},
			},
		},
	}

	containerWithKeys := apiv1.Container{
		Env: []apiv1.EnvVar{
			{
				ValueFrom: &apiv1.EnvVarSource{
					ConfigMapKeyRef: &apiv1.ConfigMapKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: "configMapWithKey",
						},
						Key: "configMapKey",
					},
				},
			},
			{
				ValueFrom: &apiv1.EnvVarSource{
					SecretKeyRef: &apiv1.SecretKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: "secretWithKey",
						},
						Key: "secretKey",
					},
				},
			},
		},
	}

	containerWithKeysButVolumeConflicts := apiv1.Container{
		Env: []apiv1.EnvVar{
			{
				ValueFrom: &apiv1.EnvVarSource{
					ConfigMapKeyRef: &apiv1.ConfigMapKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
							Name: "configMap",
						},
						Key: "configMapKey",
					},
				},
			},
			{
				ValueFrom: &apiv1.EnvVarSource{
					SecretKeyRef: &apiv1.SecretKeySelector{
						LocalObjectReference: apiv1.LocalObjectReference{
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
		input       apiv1.PodSpec
		expected    map[string][]string
	}{
		{
			description: "with Volume one secret",
			input: apiv1.PodSpec{
				Volumes: []apiv1.Volume{
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
			input: apiv1.PodSpec{
				Volumes: []apiv1.Volume{
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
			input: apiv1.PodSpec{
				Volumes: []apiv1.Volume{
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
			input: apiv1.PodSpec{
				Volumes: []apiv1.Volume{
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
			input: apiv1.PodSpec{
				Containers: []apiv1.Container{
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
			input: apiv1.PodSpec{
				Containers: []apiv1.Container{
					containerWithEnv,
					containerWithRedundantName,
				},
				Volumes: []apiv1.Volume{
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
			input: apiv1.PodSpec{
				Containers: []apiv1.Container{
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
			input: apiv1.PodSpec{
				Containers: []apiv1.Container{
					containerWithKeysButVolumeConflicts,
				},
				Volumes: []apiv1.Volume{
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
		input       []v1.Container
		expected    bool
	}{
		{
			description: "following semver",
			input: []v1.Container{
				GetContainer("test:1.0.0"),
			},
			expected: false,
		},
		{
			description: "not following semver",
			input: []v1.Container{
				GetContainer("test:latest"),
			},
			expected: true,
		},
		{
			description: "all following semver",
			input: []v1.Container{
				GetContainer("test:1.0.0"),
				GetContainer("test:1.0.0-alpha"),
				GetContainer("test:1.0.0+20130313144700"),
				GetContainer("test:1.0.0-beta+exp.sha.5114f85"),
			},
			expected: false,
		},
		{
			description: "one not following semver",
			input: []v1.Container{
				GetContainer("test:1.0.0"),
				GetContainer("test:1.0.0-alpha"),
				GetContainer("test:1.0.0+20130313144700"),
				GetContainer("test:tag1"),
			},
			expected: true,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			targetObject, err := NewResource("../deploy/testdata/test-deployment.yaml")
			utils.CheckError(err)

			podSpec := GetPodSpec(nil, &tt.input)

			targetObject.Info = GetDeploymentResource(targetObject, nil, &podSpec)
			boolRes, err := IsNotUsingSemver(targetObject)
			require.Nil(t, err)
			require.Equal(t, tt.expected, boolRes)
		})
	}
}
