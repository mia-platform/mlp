package resourceutil

import (
	"io"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
)

// InfoGenerator generates `resource.Info`
type InfoGenerator interface {
	// Generates Info from path
	FromFile(path string) ([]*resource.Info, error)

	// Generates Info from name and namespace
	FromNames(namespace string, resource string, names []string) ([]*resource.Info, error)

	// Generates Info from a stream
	FromStream(stream io.Reader) ([]*resource.Info, error)

	// Generates an Helper to interact with Kubernetes resources
	NewHelper(resource.RESTClient, *meta.RESTMapping) Helper
}

// Helper gives facilities to interact with Kubernetes resources
type Helper interface {
	Get(namespace, name string) (runtime.Object, error)
	Create(namespace string, modify bool, obj runtime.Object) (runtime.Object, error)
	Replace(namespace, name string, overwrite bool, obj runtime.Object) (runtime.Object, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, options *metav1.PatchOptions) (runtime.Object, error)
}

// Builder wraps a `resource.Builder` and implements `InfoGenerator` interface
type Builder struct {
	builder *resource.Builder
}

// FromFile use `resource.Builder` to generate a `resource.Info` from a path
func (b *Builder) FromFile(path string) ([]*resource.Info, error) {
	return b.builder.
		Path(false, path).
		Do().Infos()
}

// FromNames use `resource.Builder` to generate a `resource.Info` using resource names
// and namespace
func (b *Builder) FromNames(namespace string, resource string, names []string) ([]*resource.Info, error) {
	return b.builder.
		NamespaceParam(namespace).
		ResourceNames(resource, names...).
		Do().Infos()
}

// FromStream use `resource.Builder` to generate a `resource.Info` reading from a stream
func (b *Builder) FromStream(stream io.Reader) ([]*resource.Info, error) {
	return b.builder.
		Stream(stream, "").
		Do().Infos()
}

// NewHelper creates a new Helper
func (b *Builder) NewHelper(client resource.RESTClient, mapping *meta.RESTMapping) Helper {
	return resource.NewHelper(client, mapping)
}

// NewBuilder creates an unstructured Builder from a given config
func NewBuilder(config *genericclioptions.ConfigFlags) *Builder {
	return &Builder{
		builder: resource.NewBuilder(config).
			Unstructured().
			RequireObject(true).
			Flatten(),
	}
}
