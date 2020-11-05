package deploy

import (
	"testing"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchapiv1 "k8s.io/api/batch/v1"
	batchapiv1beta1 "k8s.io/api/batch/v1beta1"
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
		require.Nil(t, err)

		err = apply(builder, *deployment)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)

		err = apply(builder, *deployment)
		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.True(t, builder.Helper.PatchCalled)

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
		require.Nil(t, err)

		err = apply(builder, *secret)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		err = apply(builder, *secret)
		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
		require.True(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
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
		require.Nil(t, err)

		err = apply(builder, *secret)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		err = apply(builder, *secret)
		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
	})
}

func TestEnsureNamespaceExistance(t *testing.T) {
	t.Run("Ensure Namespace existance", func(t *testing.T) {
		namespaceName := "foo"
		namespace := &apiv1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespaceName},
			TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
		}
		builder := resourceutil.NewFakeBuilder()

		// This will be returned in FromStream()
		err := builder.AddResources([]runtime.Object{namespace}, false)
		require.Nil(t, err)

		actual, err := ensureNamespaceExistance(builder, namespaceName)
		require.Nil(t, err, "No errors when namespace is created")
		require.True(t, builder.Helper.CreateCalled)
		require.Equal(t, namespace, actual)

		actual, err = ensureNamespaceExistance(builder, namespaceName)
		require.Nil(t, err, "No errors when namespace already exists")
		require.True(t, builder.Helper.CreateCalled)
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

	builder := resourceutil.NewFakeBuilder()

	// This will be returned in FromStream()
	err = builder.AddResources([]runtime.Object{expected}, false)
	require.Nil(t, err)

	job, err := createJobFromCronjob(builder, *cron)
	require.Nil(t, err)
	require.True(t, builder.Helper.CreateCalled)
	require.Equal(t, expected.TypeMeta, job.TypeMeta)
	require.Equal(t, expected.ObjectMeta.Annotations, job.ObjectMeta.Annotations)
	require.Contains(t, job.ObjectMeta.GenerateName, cron.Name)
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

func TestUpdateResourceSecret(t *testing.T) {
	secret := &apiv1.Secret{
		Data: map[string][]byte{"resources": []byte(`{"CronJob":{"kind":"CronJob","Mapping":{"Group":"batch","Version":"v1beta1","Resource":"cronjobs"},"resources":["bar"]}}`)},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceSecretName,
			Namespace: "foo",
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	resources := map[string]*ResourceList{
		"CronJob": &ResourceList{
			Kind: "CronJob",
			Mapping: schema.GroupVersionResource{
				Group:    "batch",
				Version:  "v1beta1",
				Resource: "cronjobs",
			},
			Resources: []string{"bar"},
		},
	}

	t.Run("Create resource-deployed secret for the first time", func(t *testing.T) {
		builder := resourceutil.NewFakeBuilder()
		err := builder.AddResources([]runtime.Object{secret}, false)
		require.Nil(t, err)

		newSecret, err := updateResourceSecret(builder, "foo", resources)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		require.Equal(t, secret, newSecret)
	})
	t.Run("Create resource-deployed secret for the first time", func(t *testing.T) {
		builder := resourceutil.NewFakeBuilder()
		err := builder.AddResources([]runtime.Object{secret}, true)
		require.Nil(t, err)

		newSecret, err := updateResourceSecret(builder, "foo", resources)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.True(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		require.Equal(t, secret, newSecret)
	})
}

func TestPrune(t *testing.T) {
	deleteList := &ResourceList{
		Kind: "Secret",
		Mapping: schema.GroupVersionResource{
			Group:    "",
			Version:  "v1",
			Resource: "secrets",
		},
		Resources: []string{
			"foo",
		},
	}

	t.Run("Prune resource containing label", func(t *testing.T) {
		toPrune := &apiv1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				Labels: map[string]string{
					resourceutil.ManagedByLabel: resourceutil.ManagedByMia,
				},
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}

		builder := resourceutil.NewFakeBuilder()
		err := builder.AddResources([]runtime.Object{toPrune}, true)
		require.Nil(t, err)

		err = prune(builder, "bar", deleteList)
		require.Nil(t, err)
		require.True(t, builder.Helper.DeleteCalled, "the resource should be deleted")
		require.Equal(t, 0, len(builder.Helper.ClusterObjs), "the cluster should be empty")

	})

	t.Run("Don't prune resource without label", func(t *testing.T) {
		toPrune := &apiv1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}

		builder := resourceutil.NewFakeBuilder()
		err := builder.AddResources([]runtime.Object{toPrune}, true)
		require.Nil(t, err)

		err = prune(builder, "bar", deleteList)
		require.Nil(t, err)
		require.False(t, builder.Helper.DeleteCalled, "the resource should not be deleted")
		require.Equal(t, toPrune, builder.Helper.ClusterObjs[0].GetObject(), "the cluster still contains the resource")

	})
}
