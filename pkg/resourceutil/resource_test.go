package resourceutil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
)

func TestNewResource(t *testing.T) {

	builder := resource.NewBuilder(genericclioptions.NewTestConfigFlags()).
		Unstructured().
		RequireObject(true).
		Flatten()

	mockMacker := func(builder *resource.Builder, namespace string, path string) (*resource.Info, error) {
		return &resource.Info{}, nil
	}

	t.Run("Read a valid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "kubernetesersource.yaml")

		actual, err := NewResource(builder, mockMacker, "default", filePath)
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
			Info: &resource.Info{},
		}
		require.Nil(t, err, "Reading a valid k8s file err must be nil")
		require.Equal(t, actual, expected, "Resource read from file must be equal to expected")
	})

	t.Run("Read an invalid kubernetes resource", func(t *testing.T) {
		filePath := filepath.Join(testdata, "notarresource.yaml")

		resource, err := NewResource(builder, mockMacker, "", filePath)
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
