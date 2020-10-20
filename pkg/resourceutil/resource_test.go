package resourceutil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
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

// func fkf(version schema.GroupVersion) (RESTClient, error){
// 	return resource., nil
// }
// func TestMakeInfo(t *testing.T) {
// 	b := resource.NewFakeBuilder()

// 	MakeInfo(b,"default", "testdata/kubernetesresource.yaml")

// }
