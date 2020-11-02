package resourceutil

import (
	"errors"
	"fmt"
	"io"
	"net/http"
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
	Helper *MockHelper
}

// NewFakeBuilder creates a new FakeBuilder
func NewFakeBuilder() *FakeBuilder {
	return &FakeBuilder{
		Helper: &MockHelper{},
	}
}

// FromFile return and empty resource.Info array
func (b *FakeBuilder) FromFile(path string) ([]*resource.Info, error) {
	file, err := utils.ReadFile(path)
	utils.CheckError(err)
	if strings.Contains(string(file), "---\n") {
		return make([]*resource.Info, 2), nil
	}
	return []*resource.Info{
		&resource.Info{},
	}, nil
}

// FromNames return and empty resource.Info array
func (b *FakeBuilder) FromNames(namespace string, res string, names []string) ([]*resource.Info, error) {
	return []*resource.Info{
		&resource.Info{},
	}, nil
}

// FromStream return and empty resource.Info array
func (b *FakeBuilder) FromStream(stream io.Reader) ([]*resource.Info, error) {

	return []*resource.Info{
		&resource.Info{
			Name:      b.Helper.ClusterObjs[0].name,
			Namespace: b.Helper.ClusterObjs[0].namespace,
			Object:    b.Helper.ClusterObjs[0].obj,
		},
	}, nil
}

// NewHelper return a new resourceutil.MockHelper
func (b *FakeBuilder) NewHelper(client resource.RESTClient, mapping *meta.RESTMapping) Helper {
	b.Helper.CreateCalled = false
	b.Helper.PatchCalled = false
	b.Helper.ReplaceCalled = false
	return b.Helper
}

// AddResources add clusterObj to the helper
func (b *FakeBuilder) AddResources(objects []runtime.Object) error {
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

		b.Helper.ClusterObjs = append(b.Helper.ClusterObjs, cobj)
	}

	return nil
}

type clusterObj struct {
	name             string
	namespace        string
	groupVersionKind schema.GroupVersionKind
	obj              runtime.Object
}

// MockHelper mocks Helper facilities
type MockHelper struct {
	ClusterObjs      []clusterObj
	groupVersionKind schema.GroupVersionKind
	PatchCalled      bool
	ReplaceCalled    bool
	CreateCalled     bool
}

func (mh *MockHelper) Get(namespace, name string) (runtime.Object, error) {
	for _, v := range mh.ClusterObjs {
		if namespace == v.namespace && name == v.name {
			return v.obj, nil
		}
	}
	return nil, &apierrors.StatusError{metav1.Status{
		Status: metav1.StatusFailure,
		Code:   http.StatusNotFound,
		Reason: metav1.StatusReasonNotFound,
		Details: &metav1.StatusDetails{
			Group: mh.groupVersionKind.Group,
			Kind:  mh.groupVersionKind.Kind,
			Name:  name,
		},
		Message: fmt.Sprintf("%s not found", name),
	}}
}

func (mh *MockHelper) Create(namespace string, modify bool, obj runtime.Object) (runtime.Object, error) {
	mh.CreateCalled = true
	objMeta, err := meta.Accessor(obj)
	utils.CheckError(err)
	for _, v := range mh.ClusterObjs {
		if namespace == v.namespace && objMeta.GetName() == v.name {
			return obj, nil
		}
	}
	mh.ClusterObjs = append(mh.ClusterObjs, clusterObj{name: objMeta.GetName(), namespace: namespace, groupVersionKind: obj.GetObjectKind().GroupVersionKind(), obj: obj})
	return obj, nil
}

func (mh *MockHelper) Replace(namespace string, name string, overwrite bool, obj runtime.Object) (runtime.Object, error) {
	mh.ReplaceCalled = true
	for _, v := range mh.ClusterObjs {
		if namespace == v.namespace && name == v.name {
			return v.obj, nil
		}
	}
	return nil, errors.New("Object Not found")
}

func (mh *MockHelper) Patch(namespace, name string, pt types.PatchType, data []byte, options *metav1.PatchOptions) (runtime.Object, error) {
	mh.PatchCalled = true
	return nil, nil
}
