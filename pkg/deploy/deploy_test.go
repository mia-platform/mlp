package deploy

import (
	"testing"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/resource"
)

func TestApply(t *testing.T) {

	t.Run("Create and patch deployment", func(t *testing.T) {
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

		builder := resourceutil.NewFakeBuilder()
		err = builder.AddResources([]runtime.Object{deployment.Info.Object})
		require.Nil(t, err)

		err = apply(builder, *deployment)
		require.False(t, builder.Helper.ReplaceCalled)
		require.True(t, builder.Helper.PatchCalled)
		require.Nil(t, err)
		err = apply(builder, *deployment)
		require.False(t, builder.Helper.ReplaceCalled)
		require.True(t, builder.Helper.PatchCalled)
		require.Nil(t, err)

	})

	t.Run("Create and replace secret", func(t *testing.T) {
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

		builder := resourceutil.NewFakeBuilder()
		err = builder.AddResources([]runtime.Object{secret.Info.Object})
		require.Nil(t, err)

		err = apply(builder, *secret)
		require.True(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		require.Nil(t, err)
		err = apply(builder, *secret)
		require.True(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		require.Nil(t, err)

	})

	t.Run("Create a secret and not replace it", func(t *testing.T) {

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

		builder := resourceutil.NewFakeBuilder()
		err = builder.AddResources([]runtime.Object{secret.Info.Object})
		require.Nil(t, err)

		err = apply(builder, *secret)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		require.Nil(t, err)
		err = apply(builder, *secret)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
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
		builder := resourceutil.NewFakeBuilder()
		err := builder.AddResources([]runtime.Object{namespace})
		require.Nil(t, err)
		actual, err := ensureNamespaceExistance(builder, namespaceName)
		require.Nil(t, err)
		require.Equal(t, namespace, actual)
	})

	t.Run("Don't create a Namespace if already exists", func(t *testing.T) {
		namespaceName := "foo"
		namespace := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
			TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		}
		builder := resourceutil.NewFakeBuilder()
		err := builder.AddResources([]runtime.Object{namespace})
		require.Nil(t, err)

		actual, err := ensureNamespaceExistance(builder, namespaceName)
		require.Nil(t, err)
		require.Equal(t, namespace, actual)
	})
}

func TestCreatePatch(t *testing.T) {
	t.Run("Pass the same object should produce empty patch", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/aaa-test-deployent.yml")
		utils.CheckError(err)
		deployment.Info = &resource.Info{
			Object: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      deployment.Name,
					Namespace: deployment.Namespace,
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"Deployment\",\"apiVersion\":\"apps/v1\",\"metadata\":{\"name\":\"aaa-test-deployment\",\"creationTimestamp\":null},\"spec\":{\"selector\":null,\"template\":{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"containers\":null}},\"strategy\":{}},\"status\":{}}\n",
					},
				},
			},
			Namespace: deployment.Namespace,
			Name:      deployment.Name,
			Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}},
		}

		patch, patchType, err := createPatch(deployment.Info.Object, *deployment)
		require.Equal(t, []byte(`{}`), patch, "patch should be empty")
		require.Equal(t, patchType, types.StrategicMergePatchType)
		require.Nil(t, err)
	})

	t.Run("change resource name", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/aaa-test-deployent.yml")
		utils.CheckError(err)
		deploymentObject := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployment.Name,
				Namespace: deployment.Namespace,
				Annotations: map[string]string{
					"kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"Deployment\",\"apiVersion\":\"apps/v1\",\"metadata\":{\"name\":\"aaa-test-deployment\",\"creationTimestamp\":null},\"spec\":{\"selector\":null,\"template\":{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"containers\":null}},\"strategy\":{}},\"status\":{}}\n",
				},
			},
		}
		deployment.Info = &resource.Info{
			Object:    deploymentObject,
			Namespace: deployment.Namespace,
			Name:      deployment.Name,
			Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}},
		}

		var oldDeploy appsv1.Deployment
		deploymentObject.DeepCopyInto(&oldDeploy)
		oldDeploy.ObjectMeta.Name = "foo"
		patch, patchType, err := createPatch(&oldDeploy, *deployment)
		require.Equal(t, []byte(`{"metadata":{"name":"aaa-test-deployment"}}`), patch, "patch should contain the new resource name")
		require.Equal(t, patchType, types.StrategicMergePatchType)
		require.Nil(t, err)
	})
}
