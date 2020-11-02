package resourceutil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
	b := NewFakeBuilder()

	t.Run("File with two resources", func(t *testing.T) {
		_, err := MakeInfo(b, "default", "testdata/tworesources.yaml")

		require.EqualError(t, err, "Multiple objects in single yaml file currently not supported")
	})

	t.Run("resource built with correct namespace", func(t *testing.T) {
		info, err := MakeInfo(b, "default", "testdata/kubernetesersource.yaml")
		require.Nil(t, err)
		require.Equal(t, "default", info.Namespace, "Multiple objects in single yaml file currently not supported")
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
