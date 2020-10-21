package deploy

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchapiv1 "k8s.io/api/batch/v1"
	batchapiv1beta1 "k8s.io/api/batch/v1beta1"
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
	fakebatchv1 "k8s.io/client-go/kubernetes/typed/batch/v1/fake"
	fakebatchv1beta1 "k8s.io/client-go/kubernetes/typed/batch/v1beta1/fake"
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
	patchCalled      bool
	replaceCalled    bool
	createCalled     bool
}

func (mh *mockHelper) Get(namespace, name string) (runtime.Object, error) {
	for _, v := range mh.clusterObjs {
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

func (mh *mockHelper) Create(namespace string, modify bool, obj runtime.Object) (runtime.Object, error) {
	mh.createCalled = true
	objMeta, err := meta.Accessor(obj)
	utils.CheckError(err)
	for _, v := range mh.clusterObjs {
		if namespace == v.namespace && objMeta.GetName() == v.name {
			errorString := fmt.Sprintf("Creating already existing object: %s", v.name)
			return nil, errors.New(errorString)
		}
	}
	mh.clusterObjs = append(mh.clusterObjs, clusterObj{name: objMeta.GetName(), namespace: namespace, groupVersionKind: obj.GetObjectKind().GroupVersionKind(), obj: obj})
	fmt.Printf("Append to clusterObjs: %v\n", mh.clusterObjs)
	return obj, nil
}

func (mh *mockHelper) Replace(namespace string, name string, overwrite bool, obj runtime.Object) (runtime.Object, error) {
	mh.replaceCalled = true
	for _, v := range mh.clusterObjs {
		if namespace == v.namespace && name == v.name {
			return v.obj, nil
		}
	}
	return nil, errors.New("Object Not found")
}

func (mh *mockHelper) Patch(namespace, name string, pt types.PatchType, data []byte, options *metav1.PatchOptions) (runtime.Object, error) {
	mh.patchCalled = true
	return nil, nil
}

func TestApply(t *testing.T) {

	t.Run("Create and patch deployment", func(t *testing.T) {
		helper := &mockHelper{}
		deployment, err := resourceutil.NewResource("testdata/aaa-test-deployent.yml")
		utils.CheckError(err)
		deployment.Info = &resource.Info{
			Object: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      deployment.Name,
					Namespace: deployment.Namespace,
				},
			},
			Namespace: deployment.Namespace,
			Name:      deployment.Name,
			Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}},
		}
		err = apply(*deployment, helper)
		require.True(t, helper.createCalled)
		require.False(t, helper.replaceCalled)
		require.False(t, helper.patchCalled)
		require.Nil(t, err)

		err = apply(*deployment, helper)
		require.True(t, helper.createCalled)
		require.False(t, helper.replaceCalled)
		require.True(t, helper.patchCalled)
		require.Nil(t, err)

	})

	t.Run("Create and replace secret", func(t *testing.T) {
		helper := &mockHelper{}
		secret, err := resourceutil.NewResource("testdata/opaque.secret.yaml")
		utils.CheckError(err)

		secret.Info = &resource.Info{
			Object: &apiv1.Secret{
				Type: apiv1.SecretTypeOpaque,
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				},
				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			},
			Namespace: secret.Namespace,
			Name:      secret.Name,
		}
		err = apply(*secret, helper)
		require.True(t, helper.createCalled)
		require.False(t, helper.replaceCalled)
		require.False(t, helper.patchCalled)
		require.Nil(t, err)
		err = apply(*secret, helper)
		require.True(t, helper.createCalled)
		require.True(t, helper.replaceCalled)
		require.False(t, helper.patchCalled)
		require.Nil(t, err)

	})

	t.Run("Create a secret and not replace it", func(t *testing.T) {
		helper := &mockHelper{}
		secret, err := resourceutil.NewResource("testdata/tls.secret.yaml")
		utils.CheckError(err)

		secret.Info = &resource.Info{
			Object: &apiv1.Secret{
				Type: apiv1.SecretTypeTLS,
				ObjectMeta: metav1.ObjectMeta{
					Name:      secret.Name,
					Namespace: secret.Namespace,
				},
				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			},
			Namespace: secret.Namespace,
			Name:      secret.Name,
		}
		err = apply(*secret, helper)
		require.True(t, helper.createCalled)
		require.False(t, helper.replaceCalled)
		require.False(t, helper.patchCalled)
		require.Nil(t, err)
		err = apply(*secret, helper)
		require.True(t, helper.createCalled)
		require.False(t, helper.replaceCalled)
		require.False(t, helper.patchCalled)
		require.Nil(t, err)
	})
}

func TestEnsureNamespaceExistance(t *testing.T) {
	t.Run("Create a Namespace if does not exists", func(t *testing.T) {
		namespaceName := "foo"
		namespace := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
			TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		}
		client = fake.NewSimpleClientset()
		client.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("get", "namespaces", func(action faketesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, &v1.Namespace{}, apierrors.NewNotFound(schema.GroupResource{Group: "v1", Resource: "Namespace"}, "foo")
		})
		client.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("create", "namespaces", func(action faketesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, namespace, nil
		})
		actual, err := ensureNamespaceExistance(client, namespaceName)
		require.Nil(t, err)
		require.Equal(t, namespace, actual)
	})

	t.Run("Don't create a Namespace if already exists", func(t *testing.T) {
		namespaceName := "foo"
		namespace := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
			TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		}
		client := fake.NewSimpleClientset()
		client.CoreV1().(*fakecorev1.FakeCoreV1).PrependReactor("get", "namespace", func(action faketesting.Action) (handled bool, ret runtime.Object, err error) {
			return true, namespace, nil
		})
		actual, err := ensureNamespaceExistance(client, namespaceName)
		require.Nil(t, err)
		require.Equal(t, namespace, actual)
	})
}

func TestCreateJobFromCronJob(t *testing.T) {
	cron, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
	utils.CheckError(err)
	cron.Info = &resource.Info{
		Object: &batchapiv1beta1.CronJob{
			TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1beta1", Kind: "CronJob"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      cron.Name,
				Namespace: cron.Namespace,
			},
		},
		Namespace: cron.Namespace,
		Name:      cron.Name,
		Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "batch", Version: "v1beta1", Kind: "CronJob"}},
	}

	expected := &batchapiv1.Job{
		TypeMeta: metav1.TypeMeta{APIVersion: batchapiv1.SchemeGroupVersion.String(), Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: cron.Name + "-",
			Annotations: map[string]string{
				"cronjob.kubernetes.io/instantiate": "manual",
			},
		},
	}

	client := fake.NewSimpleClientset()
	client.BatchV1beta1().(*fakebatchv1beta1.FakeBatchV1beta1).PrependReactor("get", "cronjobs", func(action faketesting.Action) (handled bool, ret runtime.Object, err error) {
		return true, cron.Info.Object, nil
	})
	client.BatchV1().(*fakebatchv1.FakeBatchV1).PrependReactor("create", "jobs", func(action faketesting.Action) (handled bool, ret runtime.Object, err error) {
		return false, expected, nil
	})
	job, err := createJobFromCronjob(client, *cron)

	require.Equal(t, expected.TypeMeta, job.TypeMeta)
	require.Equal(t, expected.ObjectMeta.Annotations, job.ObjectMeta.Annotations)
	require.Contains(t, job.ObjectMeta.GenerateName, cron.Name)
	require.Nil(t, err)
}
