package resourceutil

import (
	"io"
	"strings"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/resource"
)

// FakeBuilder mocks resourceutil.Builder
type FakeBuilder struct {
	Resources []*clusterObj
	Helper    *MockHelper
}

// NewFakeBuilder creates a new FakeBuilder
func NewFakeBuilder() *FakeBuilder {
	return &FakeBuilder{
		Helper: &MockHelper{},
	}
}

// FromFile returns an empty resource.Info array if the file contains multiple
// resources. Otherwise return the first element in `ClusterObjs`
func (b *FakeBuilder) FromFile(path string) ([]*resource.Info, error) {
	file, err := utils.ReadFile(path)
	utils.CheckError(err)

	// Return empty array since multiple resource in a single file
	// are currently not supported
	if strings.Contains(string(file), "---\n") {
		return make([]*resource.Info, 2), nil
	}
	return b.returnFirstObject()
}

// FromNames returns the first object in `mockHelper.ClusterObjs`
func (b *FakeBuilder) FromNames(namespace string, res string, names []string) ([]*resource.Info, error) {
	return b.returnFirstObject()

}

// FromStream returns the first object in `mockHelper.ClusterObjs`
func (b *FakeBuilder) FromStream(stream io.Reader) ([]*resource.Info, error) {
	return b.returnFirstObject()
}

func (b *FakeBuilder) returnFirstObject() ([]*resource.Info, error) {
	if len(b.Resources) == 0 {
		return []*resource.Info{&resource.Info{}}, nil
	}

	return []*resource.Info{
		&resource.Info{
			Name:      b.Resources[0].name,
			Namespace: b.Resources[0].namespace,
			Object:    b.Resources[0].obj,
		},
	}, nil
}

// NewHelper return the existing resourceutil.MockHelper and set all the flags to false
func (b *FakeBuilder) NewHelper(client resource.RESTClient, mapping *meta.RESTMapping) Helper {
	b.Helper.CreateCalled = false
	b.Helper.PatchCalled = false
	b.Helper.ReplaceCalled = false
	b.Helper.DeleteCalled = false
	return b.Helper
}

// AddResources add clusterObj to the Builder and optionally to the helper
func (b *FakeBuilder) AddResources(objects []runtime.Object, addToHelper bool) error {
	for _, obj := range objects {
		objectMeta, err := meta.Accessor(obj)
		if err != nil {
			return err
		}
		cobj := clusterObj{
			name:             objectMeta.GetName(),
			namespace:        objectMeta.GetNamespace(),
			groupVersionKind: obj.GetObjectKind().GroupVersionKind(),
			obj:              obj,
		}

		b.Resources = append(b.Resources, &cobj)

		if addToHelper {
			b.Helper.ClusterObjs = append(b.Helper.ClusterObjs, &cobj)
		}
	}

	return nil
}

type clusterObj struct {
	name             string
	namespace        string
	groupVersionKind schema.GroupVersionKind
	obj              runtime.Object
}

func (c clusterObj) GetObject() runtime.Object {
	return c.obj
}

// MockHelper mocks Helper facilities
type MockHelper struct {
	ClusterObjs      []*clusterObj
	groupVersionKind schema.GroupVersionKind
	PatchCalled      bool
	ReplaceCalled    bool
	CreateCalled     bool
	DeleteCalled     bool
}

// Get the object if present in `ClusterObjs`
func (mh *MockHelper) Get(namespace, name string) (runtime.Object, error) {
	for _, v := range mh.ClusterObjs {
		if namespace == v.namespace && name == v.name {
			return v.obj, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, name)
}

// Create appends the object in `ClusterObjs`
func (mh *MockHelper) Create(namespace string, modify bool, obj runtime.Object) (runtime.Object, error) {
	mh.CreateCalled = true
	objMeta, err := meta.Accessor(obj)

	if err != nil {
		return nil, err
	}

	for _, v := range mh.ClusterObjs {
		if namespace == v.namespace && objMeta.GetName() == v.name {
			return nil, apierrors.NewAlreadyExists(schema.GroupResource{}, v.name)
		}
	}
	mh.ClusterObjs = append(mh.ClusterObjs, &clusterObj{name: objMeta.GetName(), namespace: namespace, groupVersionKind: obj.GetObjectKind().GroupVersionKind(), obj: obj})
	return obj, nil
}

// Replace changes the object stored in `ClusterObj`
func (mh *MockHelper) Replace(namespace string, name string, overwrite bool, obj runtime.Object) (runtime.Object, error) {
	mh.ReplaceCalled = true
	for _, v := range mh.ClusterObjs {
		if namespace == v.namespace && name == v.name {
			v.obj = obj
			return v.obj, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, name)
}

// Patch not implemented
func (mh *MockHelper) Patch(namespace, name string, pt types.PatchType, data []byte, options *metav1.PatchOptions) (runtime.Object, error) {
	mh.PatchCalled = true
	return nil, nil
}

// Delete the object stored in `ClusterObj`
func (mh *MockHelper) Delete(namespace, name string) (runtime.Object, error) {
	mh.DeleteCalled = true
	for i, v := range mh.ClusterObjs {
		if namespace == v.namespace && name == v.name {
			mh.ClusterObjs = append(mh.ClusterObjs[:i], mh.ClusterObjs[i+1:]...)
			return v.obj, nil
		}
	}
	return nil, apierrors.NewNotFound(schema.GroupResource{}, name)
}
