package deploy

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/client-go/kubernetes/fake"
	fakecorev1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	faketesting "k8s.io/client-go/testing"
)

type clusterObj struct {
	name             string
	namespace        string
	groupVersionKind schema.GroupVersionKind
	obj              runtime.Object
}

type mockHelper struct {
	clusterObjs      []clusterObj
	groupVersionKind schema.GroupVersionKind
}

func (mh mockHelper) Get(namespace, name string) (runtime.Object, error) {
	for _, v := range mh.clusterObjs {
		if namespace == v.namespace && name == v.name && mh.groupVersionKind == v.groupVersionKind {
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

func (mh mockHelper) Create(namespace string, modify bool, obj runtime.Object) (runtime.Object, error) {
	objMeta, err := meta.Accessor(obj)
	utils.CheckError(err)
	for _, v := range mh.clusterObjs {
		if namespace == v.namespace && objMeta.GetName() == v.name && mh.groupVersionKind == v.groupVersionKind {
			errorString := fmt.Sprintf("Creating already existing object: %s", v.name)
			return nil, errors.New(errorString)
		}
	}
	mh.clusterObjs = append(mh.clusterObjs, clusterObj{name: objMeta.GetName(), namespace: namespace, groupVersionKind: obj.GetObjectKind().GroupVersionKind(), obj: obj})
	fmt.Printf("Append to clusterObjs: %v\n", mh.clusterObjs)
	return obj, nil
}

func (mh mockHelper) Replace(namespace string, name string, overwrite bool, obj runtime.Object) (runtime.Object, error) {
	for _, v := range mh.clusterObjs {
		if namespace == v.namespace && name == v.name && mh.groupVersionKind == v.groupVersionKind {
			return v.obj, nil
		}
	}
	return nil, errors.New("Object Not found")
}

func (mh mockHelper) Patch(namespace, name string, pt types.PatchType, data []byte, options *metav1.PatchOptions) (runtime.Object, error) {
	return nil, nil
}

var randomObj runtime.Object = &apiv1.Secret{
	Type: apiv1.SecretTypeDockerConfigJson,
	ObjectMeta: metav1.ObjectMeta{
		Name: "foo",
	},
	TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
}

func mockAddInfos(oldresources []resourceutil.Resource) []resourceutil.Resource {
	resources := []resourceutil.Resource{}
	for _, res := range oldresources {
		res.Info = &resource.Info{}
		res.Info.Object = randomObj
		res.Info.Namespace = "default" // TODO check this out
		res.Info.Name = res.Name
		gv, err := schema.ParseGroupVersion(res.Head.GroupVersion)
		utils.CheckError(err)
		gvk := gv.WithKind(res.Head.Kind)
		res.Info.Object.GetObjectKind().SetGroupVersionKind(gvk)
		resources = append(resources, res)
	}
	return resources
}

// func TestApply(t *testing.T) {
// 	path := "testdata/"
// 	t.Run()
// }

func TestCreatingResources(t *testing.T) {
	expectedMetadata := struct {
		Name        string            `json:"name"`
		Annotations map[string]string `json:"annotations"`
	}{
		Name: "literal",
	}

	testcases := []resourceutil.Resource{
		{
			Filepath: "testdata/files-configmap.yaml",
			Name:     "literal",
			Head: resourceutil.ResourceHead{
				GroupVersion: "v1",
				Kind:         "ConfigMap",
				Metadata:     &expectedMetadata,
			},
		},
	}

	resources := mockAddInfos(testcases)
	// Mocking deploy
	helper := mockHelper{}
	for _, res := range resources {
		helper.groupVersionKind = res.Info.Object.GetObjectKind().GroupVersionKind()
		err := apply(res, helper)
		require.Nil(t, err)
		err = apply(res, helper)
		require.Nil(t, err)
	}
}

func TestEnsureNamespaceExistance(t *testing.T) {
	t.Run("Create a Namespace if does not exists", func(t *testing.T) {
		namespace := "foo"
		client := fake.NewSimpleClientset(&apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
			TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		})
		client.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("get", "namespace", func(action faketesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, &v1.Namespace{}, apierrors.NewNotFound(schema.GroupResource{Group: "v1", Resource: "Namespace"}, "foo")
		})
		client.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("create", "namespace", func(action faketesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, &v1.Namespace{}, nil
		})
		require.Nil(t, ensureNamespaceExistance(client, namespace))
	})

	// t.Run("Don't create a Namespace if already exists", func(t *testing.T) {
	// 	namespace := "foo"
	// 	client := fake.NewSimpleClientset()
	// 	client.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("get", "namespace", func(action faketesting.Action) (handled bool, ret runtime.Object, err error) {
	// 		return true, &v1.Namespace{}, nil
	// 	})
	// 	err := ensureNamespaceExistance(client, namespace)
	// 	require.Nil(t, err)
	// })
}

func TestCreateJobFromCronJob(t *testing.T) {

}
