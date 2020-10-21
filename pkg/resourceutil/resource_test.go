package resourceutil

import (
	"path/filepath"
	"strings"
	"testing"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"

	"github.com/stretchr/testify/require"
)

type FakeBuilder struct {
	builder *resource.Builder
}

func (b *FakeBuilder) Generate(path string) ([]*resource.Info, error) {
	file, err := utils.ReadFile(path)
	utils.CheckError(err)
	if strings.Contains(string(file), "---\n") {
		return make([]*resource.Info, 2), nil
	}
	return []*resource.Info{
		&resource.Info{},
	}, nil
}

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
	b := &FakeBuilder{
		builder: resource.NewBuilder(genericclioptions.NewTestConfigFlags()),
	}

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
