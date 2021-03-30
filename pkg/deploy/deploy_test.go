package deploy

import (
	"testing"
	"time"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchapiv1 "k8s.io/api/batch/v1"
	batchapiv1beta1 "k8s.io/api/batch/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/resource"
)

func TestApply(t *testing.T) {

	deployConfig := utils.DeployConfig{
		DeployType:              deployAll,
		ForceDeployWhenNoSemver: true,
	}

	t.Run("Create and patch deployment", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)
		deployment.Info = resourceutil.GetDeploymentResource(deployment, nil, nil)

		builder := resourceutil.NewFakeBuilder()
		require.Nil(t, err)

		err = apply(builder, *deployment, deployConfig)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)

		err = apply(builder, *deployment, deployConfig)
		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.True(t, builder.Helper.PatchCalled)
	})

	t.Run("Create and replace secret", func(t *testing.T) {
		secret, err := resourceutil.NewResource("testdata/opaque.secret.yaml")
		utils.CheckError(err)

		secretType := apiv1.SecretTypeOpaque
		secret.Info = resourceutil.GetSecretResource(secret, &secretType)

		builder := resourceutil.NewFakeBuilder()
		require.Nil(t, err)

		err = apply(builder, *secret, deployConfig)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		err = apply(builder, *secret, deployConfig)
		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
		require.True(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
	})

	t.Run("Create a secret and not replace it", func(t *testing.T) {

		secret, err := resourceutil.NewResource("testdata/tls.secret.yaml")
		utils.CheckError(err)

		secretType := apiv1.SecretTypeTLS
		secret.Info = resourceutil.GetSecretResource(secret, &secretType)

		builder := resourceutil.NewFakeBuilder()
		require.Nil(t, err)

		err = apply(builder, *secret, deployConfig)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
		err = apply(builder, *secret, deployConfig)
		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)
	})
}

func TestSemVerIntegration(t *testing.T) {
	newDeployConfig := utils.DeployConfig{
		DeployType:              smartDeploy,
		ForceDeployWhenNoSemver: true,
	}

	t.Run("SmartDeploy not following semver", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)
		podSpec := resourceutil.GetPodSpec(nil, &[]v1.Container{
			resourceutil.GetContainer("test:latest"),
		})
		deployment.Info = resourceutil.GetDeploymentResource(deployment, nil, &podSpec)
		builder := resourceutil.NewFakeBuilder()
		require.Nil(t, err)

		err = apply(builder, *deployment, newDeployConfig)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)

		err = apply(builder, *deployment, newDeployConfig)
		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.True(t, builder.Helper.PatchCalled)

		newDeployment, _ := deployment.Info.Object.(*appsv1.Deployment)
		annotations := newDeployment.Spec.Template.ObjectMeta.Annotations
		require.Equal(t, "", annotations["not-exist-annotation"])
		require.NotEqual(t, "", annotations[resourceutil.GetMiaAnnotation(deployChecksum)])
	})

	t.Run("SmartDeploy following semver without original", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)
		podSpec := resourceutil.GetPodSpec(nil, &[]v1.Container{
			resourceutil.GetContainer("test:1.0.0"),
		})
		deployment.Info = resourceutil.GetDeploymentResource(deployment, nil, &podSpec)
		builder := resourceutil.NewFakeBuilder()
		require.Nil(t, err)

		err = apply(builder, *deployment, newDeployConfig)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.False(t, builder.Helper.PatchCalled)

		err = apply(builder, *deployment, newDeployConfig)
		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
		require.False(t, builder.Helper.ReplaceCalled)
		require.True(t, builder.Helper.PatchCalled)

		newDeployment, _ := deployment.Info.Object.(*appsv1.Deployment)
		annotations := newDeployment.Spec.Template.ObjectMeta.Annotations
		require.Equal(t, "", annotations["not-exist-annotation"])
		require.Equal(t, "", annotations[resourceutil.GetMiaAnnotation(deployChecksum)])
	})
}

func TestAutoCreateIntegration(t *testing.T) {
	deployConfig := utils.DeployConfig{
		DeployType:              smartDeploy,
		ForceDeployWhenNoSemver: true,
	}

	t.Run("Smart Deploy - Deployment ", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)

		builder := resourceutil.NewFakeBuilder()

		annotations := map[string]string{"test1": "test1"}
		deployment.Info = resourceutil.GetDeploymentResource(deployment, annotations, nil)

		err = builder.AddResources([]runtime.Object{deployment.Info.Object}, true)
		require.Nil(t, err)
		err = apply(builder, *deployment, deployConfig)

		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
	})

	t.Run("Smart Deploy - CronJobs with updates", func(t *testing.T) {
		cronJob, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
		utils.CheckError(err)
		annotations := map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"CronJob\",\"apiVersion\":\"batch/v1beta1\",\"metadata\":{\"name\":\"hello\",\"creationTimestamp\":null, \"annotations\": {\"injectedAnn\":\"checksum\"}},\"spec\":{\"schedule\":\"\",\"jobTemplate\":{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"template\":{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"containers\":null}}}}},\"status\":{}}\n",
		}
		cronJob.Info = &resource.Info{
			Object: &batchapiv1beta1.CronJob{
				TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1beta1", Kind: "CronJob"},
				ObjectMeta: metav1.ObjectMeta{
					Name:        cronJob.Name,
					Namespace:   cronJob.Namespace,
					Annotations: annotations,
				},
			},
			Namespace: cronJob.Namespace,
			Name:      cronJob.Name,
			Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}},
		}

		builder := resourceutil.NewFakeBuilder()
		cronJobToTest := &batchapiv1beta1.CronJob{
			TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1beta1", Kind: "CronJob"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fill first element of array",
				Namespace: cronJob.Namespace,
			},
		}

		err = builder.AddResources([]runtime.Object{cronJobToTest}, false)
		require.Nil(t, err)

		err = builder.AddResources([]runtime.Object{cronJob.Info.Object}, true)
		require.Nil(t, err)

		err = apply(builder, *cronJob, deployConfig)

		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
	})

	t.Run("Smart Deploy - CronJobs without updates", func(t *testing.T) {
		cronJob, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
		utils.CheckError(err)
		annotations := map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"CronJob\",\"apiVersion\":\"batch/v1beta1\",\"metadata\":{\"name\":\"hello\",\"creationTimestamp\":null},\"spec\":{\"schedule\":\"\",\"jobTemplate\":{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"template\":{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"containers\":null}}}}},\"status\":{}}\n",
		}
		cronJob.Info = &resource.Info{
			Object: &batchapiv1beta1.CronJob{
				TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1beta1", Kind: "CronJob"},
				ObjectMeta: metav1.ObjectMeta{
					Name:        cronJob.Name,
					Namespace:   cronJob.Namespace,
					Annotations: annotations,
				},
			},
			Namespace: cronJob.Namespace,
			Name:      cronJob.Name,
			Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}},
		}

		builder := resourceutil.NewFakeBuilder()
		cronJobToTest := &batchapiv1beta1.CronJob{
			TypeMeta: metav1.TypeMeta{APIVersion: "batch/v1beta1", Kind: "CronJob"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fill first element of array",
				Namespace: cronJob.Namespace,
			},
		}

		err = builder.AddResources([]runtime.Object{cronJobToTest}, false)
		require.Nil(t, err)

		err = builder.AddResources([]runtime.Object{cronJob.Info.Object}, true)
		require.Nil(t, err)

		err = apply(builder, *cronJob, deployConfig)

		require.Nil(t, err)
		require.False(t, builder.Helper.CreateCalled)
	})

	t.Run("Deploy All - Always create new job", func(t *testing.T) {
		cronJob, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
		cronJob.Name = "existing job"
		utils.CheckError(err)
		annotations := map[string]string{}
		cronJob.Info = resourceutil.GetCronJobResource(cronJob, annotations, nil)

		builder := resourceutil.NewFakeBuilder()
		job := &batchapiv1.Job{
			TypeMeta: metav1.TypeMeta{APIVersion: batchapiv1.SchemeGroupVersion.String(), Kind: "Job"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "new job",
				Annotations: map[string]string{
					"cronjob.kubernetes.io/instantiate": "manual",
				},
			},
		}

		job2 := &batchapiv1.Job{
			TypeMeta: metav1.TypeMeta{APIVersion: batchapiv1.SchemeGroupVersion.String(), Kind: "Job"},
			ObjectMeta: metav1.ObjectMeta{
				Name: "existing job",
				Annotations: map[string]string{
					"cronjob.kubernetes.io/instantiate": "manual",
				},
			},
		}
		err = builder.AddResources([]runtime.Object{job}, false)
		require.Nil(t, err)

		err = builder.AddResources([]runtime.Object{job2}, true)
		require.Nil(t, err)

		newDeployConfig := utils.DeployConfig{
			DeployType:              deployAll,
			ForceDeployWhenNoSemver: true,
		}
		err = apply(builder, *cronJob, newDeployConfig)
		require.Nil(t, err)
		require.True(t, builder.Helper.CreateCalled)
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
	cron.Info = resourceutil.GetCronJobResource(cron, nil, nil)

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
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)
		deployment.Info = &resource.Info{
			Object: &appsv1.Deployment{
				TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
				ObjectMeta: metav1.ObjectMeta{
					Name:      deployment.Name,
					Namespace: deployment.Namespace,
					Annotations: map[string]string{
						"kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"Deployment\",\"apiVersion\":\"apps/v1\",\"metadata\":{\"name\":\"test-deployment\",\"creationTimestamp\":null},\"spec\":{\"selector\":null,\"template\":{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"containers\":null}},\"strategy\":{}},\"status\":{}}\n",
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
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)
		deploymentObject := &appsv1.Deployment{
			TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      deployment.Name,
				Namespace: deployment.Namespace,
				Annotations: map[string]string{
					"kubectl.kubernetes.io/last-applied-configuration": "{\"kind\":\"Deployment\",\"apiVersion\":\"apps/v1\",\"metadata\":{\"name\":\"test-deployment\",\"creationTimestamp\":null},\"spec\":{\"selector\":null,\"template\":{\"metadata\":{\"creationTimestamp\":null},\"spec\":{\"containers\":null}},\"strategy\":{}},\"status\":{}}\n",
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
		require.Equal(t, []byte(`{"metadata":{"name":"test-deployment"}}`), patch, "patch should contain the new resource name")
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

	t.Run("Skip non existing resource", func(t *testing.T) {
		toPrune := &apiv1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "other-name",
				Namespace: "bar",
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}

		builder := resourceutil.NewFakeBuilder()
		err := builder.AddResources([]runtime.Object{toPrune}, true)
		require.Nil(t, err)

		resourceCount := len(builder.Helper.ClusterObjs)
		err = prune(builder, "bar", deleteList)
		require.Nil(t, err)
		require.False(t, builder.Helper.DeleteCalled, "No resources should be pruned")
		require.Equal(t, resourceCount, len(builder.Helper.ClusterObjs), "the cluster should contain the same resources after the prune of a non existing resource")

	})
}

func TestEnsureDeployAll(t *testing.T) {

	mockTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	expectedCheckSum := "6ab733c74e26e73bca78aa9c4c9db62664f339d9eefac51dd503c9ff0cf0c329"

	t.Run("Add deployment annotation", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)
		annotations := map[string]string{}
		deployment.Info = resourceutil.GetDeploymentResource(deployment, annotations, nil)

		err = ensureDeployAll(deployment, mockTime)

		newDeployment, _ := deployment.Info.Object.(*appsv1.Deployment)
		annotations = newDeployment.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}, annotations)
	})

	t.Run("Deployment without meta", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)
		deployment.Info = resourceutil.GetDeploymentResource(deployment, nil, nil)

		err = ensureDeployAll(deployment, mockTime)
		newDeployment, _ := deployment.Info.Object.(*appsv1.Deployment)
		annotations := newDeployment.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}, annotations)
	})

	t.Run("Add cronJob annotation", func(t *testing.T) {
		cronJob, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
		utils.CheckError(err)

		annotations := map[string]string{}
		cronJob.Info = resourceutil.GetCronJobResource(cronJob, annotations, nil)

		err = ensureDeployAll(cronJob, mockTime)

		newCronJob, _ := cronJob.Info.Object.(*batchapiv1beta1.CronJob)
		annotations = newCronJob.Spec.JobTemplate.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}, annotations)
	})

	t.Run("CronJob without ObjectMeta", func(t *testing.T) {
		cronJob, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
		utils.CheckError(err)

		cronJob.Info = resourceutil.GetCronJobResource(cronJob, nil, nil)

		err = ensureDeployAll(cronJob, mockTime)

		newCronJob, _ := cronJob.Info.Object.(*batchapiv1beta1.CronJob)
		annotations := newCronJob.Spec.JobTemplate.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}, annotations)
	})

	t.Run("Reject cronjob misplaced with deployment", func(t *testing.T) {
		cronJob, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
		utils.CheckError(err)

		cronJob.Info = resourceutil.GetDeploymentResource(cronJob, nil, nil)
		err = ensureDeployAll(cronJob, mockTime)

		require.Equal(t, "resource hello: not a valid cronJob", err.Error())
	})
}

func TestPrepareResources(t *testing.T) {
	mockTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	expectedCheckSum := "6ab733c74e26e73bca78aa9c4c9db62664f339d9eefac51dd503c9ff0cf0c329"

	t.Run("With deploy all and a single resource", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		utils.CheckError(err)

		resources := make([]resourceutil.Resource, 1)
		annotations := map[string]string{}
		deployment.Info = resourceutil.GetDeploymentResource(deployment, annotations, nil)

		resources[0] = *deployment

		err = prepareResources(deployAll, resources, mockTime)

		newDeployment, _ := resources[0].Info.Object.(*appsv1.Deployment)
		annotations = newDeployment.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, expectedCheckSum, annotations[resourceutil.GetMiaAnnotation(deployChecksum)])
	})

	t.Run("With deploy all and a secret", func(t *testing.T) {
		secret, err := resourceutil.NewResource("testdata/opaque.secret.yaml")
		utils.CheckError(err)

		secretType := apiv1.SecretTypeOpaque
		secret.Info = resourceutil.GetSecretResource(secret, &secretType)
		resources := make([]resourceutil.Resource, 1)

		resources[0] = *secret

		err = prepareResources(deployAll, resources, mockTime)

		require.Nil(t, err)
	})
}

func TestEnsureSmartDeploy(t *testing.T) {
	expectedCheckSum := "6ab733c74e26e73bca78aa9c4c9db62664f339d9eefac51dd503c9ff0cf0c329"

	t.Run("Add deployment deploy/checksum annotation", func(t *testing.T) {
		targetObject, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		builder := resourceutil.NewFakeBuilder()

		utils.CheckError(err)

		annotations := map[string]string{}
		targetObject.Info = resourceutil.GetDeploymentResource(targetObject, annotations, nil)

		annotations = map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}
		currentObject := resourceutil.GetDeploymentResource(targetObject, annotations, nil).Object

		err = ensureSmartDeploy(builder, currentObject, targetObject)

		newTargetObject, _ := targetObject.Info.Object.(*appsv1.Deployment)
		annotations = newTargetObject.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}, annotations)
	})

	t.Run("Add deployment without deploy/checksum annotation", func(t *testing.T) {
		targetObject, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		builder := resourceutil.NewFakeBuilder()

		utils.CheckError(err)

		annotations := map[string]string{"test": "test"}
		targetObject.Info = resourceutil.GetDeploymentResource(targetObject, annotations, nil)
		currentObject := resourceutil.GetDeploymentResource(targetObject, nil, nil).Object

		err = ensureSmartDeploy(builder, currentObject, targetObject)

		newTargetObject, _ := targetObject.Info.Object.(*appsv1.Deployment)
		annotations = newTargetObject.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{"test": "test"}, annotations)
	})

	t.Run("Add cronjob deploy/checksum annotation", func(t *testing.T) {
		targetObject, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
		utils.CheckError(err)

		builder := resourceutil.NewFakeBuilder()
		job := &batchapiv1.Job{
			TypeMeta: metav1.TypeMeta{APIVersion: batchapiv1.SchemeGroupVersion.String(), Kind: "Job"},
			ObjectMeta: metav1.ObjectMeta{
				Name: targetObject.Name,
				Annotations: map[string]string{
					"cronjob.kubernetes.io/instantiate": "manual",
				},
			},
		}
		err = builder.AddResources([]runtime.Object{job}, false)
		utils.CheckError(err)

		annotations := map[string]string{}
		targetObject.Info = resourceutil.GetCronJobResource(targetObject, annotations, nil)

		annotations = map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}
		currentObject := resourceutil.GetCronJobResource(targetObject, annotations, nil).Object

		err = ensureSmartDeploy(builder, currentObject, targetObject)

		newTargetObject, _ := targetObject.Info.Object.(*batchapiv1beta1.CronJob)
		annotations = newTargetObject.Spec.JobTemplate.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}, annotations)
	})

	t.Run("Pass default annotation", func(t *testing.T) {
		targetObject, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		builder := resourceutil.NewFakeBuilder()

		utils.CheckError(err)
		annotations := map[string]string{"test1": "test1"}
		targetObject.Info = resourceutil.GetDeploymentResource(targetObject, annotations, nil)
		annotations = map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}
		currentObject := resourceutil.GetDeploymentResource(targetObject, annotations, nil).Object

		err = ensureSmartDeploy(builder, currentObject, targetObject)

		newTargetObject, _ := targetObject.Info.Object.(*appsv1.Deployment)
		annotations = newTargetObject.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum, "test1": "test1"}, annotations)
	})
}

func TestInsertDependencies(t *testing.T) {
	var configMapMap = map[string]string{"configMap1": "aaa", "configMap2": "bbb", "configMapLongLoongLoooooooooooooooooooooooooooooooooooooooooooooong": "eee"}
	var secretMap = map[string]string{"secret1": "ccc", "secret2": "ddd"}
	testVolumes := []v1.Volume{
		{
			VolumeSource: apiv1.VolumeSource{
				Secret: &apiv1.SecretVolumeSource{
					SecretName: "secret1",
				},
			},
		},
		{
			VolumeSource: apiv1.VolumeSource{
				Secret: &apiv1.SecretVolumeSource{
					SecretName: "secret2",
				},
			},
		},
		{
			VolumeSource: apiv1.VolumeSource{
				ConfigMap: &apiv1.ConfigMapVolumeSource{
					LocalObjectReference: apiv1.LocalObjectReference{
						Name: "configMap1",
					},
				},
			},
		},
		{
			VolumeSource: apiv1.VolumeSource{
				ConfigMap: &apiv1.ConfigMapVolumeSource{
					LocalObjectReference: apiv1.LocalObjectReference{
						Name: "configMap2",
					},
				},
			},
		},
		{
			VolumeSource: apiv1.VolumeSource{
				ConfigMap: &apiv1.ConfigMapVolumeSource{
					LocalObjectReference: apiv1.LocalObjectReference{
						Name: "configMapLongLoongLoooooooooooooooooooooooooooooooooooooooooooooong",
					},
				},
			},
		},
	}

	t.Run("Test Deployment", func(t *testing.T) {
		deployment, err := resourceutil.NewResource("testdata/test-deployment.yaml")
		require.Nil(t, err)
		podSpec := resourceutil.GetPodSpec(&testVolumes, nil)
		deployment.Info = resourceutil.GetDeploymentResource(deployment, nil, &podSpec)

		err = insertDependencies(deployment, configMapMap, secretMap)
		require.Nil(t, err)
		newDeployment, _ := deployment.Info.Object.(*appsv1.Deployment)
		annotations := newDeployment.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{
			resourceutil.GetMiaAnnotation(dependenciesChecksum): "{\"configMap1-configmap\":\"aaa\",\"configMap2-configmap\":\"bbb\",\"configMapLongLoongLoooooooooooooooooooooooooooooooooooooooooooooong-configmap\":\"eee\",\"secret1-secret\":\"ccc\",\"secret2-secret\":\"ddd\"}",
		}, annotations)
	})

	t.Run("Test CronJob", func(t *testing.T) {
		cronJob, err := resourceutil.NewResource("testdata/cronjob-test.cronjob.yml")
		require.Nil(t, err)
		podSpec := resourceutil.GetPodSpec(&testVolumes, nil)
		cronJob.Info = resourceutil.GetCronJobResource(cronJob, nil, &podSpec)

		err = insertDependencies(cronJob, configMapMap, secretMap)
		require.Nil(t, err)

		newCronJob, _ := cronJob.Info.Object.(*batchapiv1beta1.CronJob)
		annotations := newCronJob.Spec.JobTemplate.Spec.Template.ObjectMeta.Annotations
		require.Nil(t, err)
		require.Equal(t, map[string]string{
			resourceutil.GetMiaAnnotation(dependenciesChecksum): "{\"configMap1-configmap\":\"aaa\",\"configMap2-configmap\":\"bbb\",\"configMapLongLoongLoooooooooooooooooooooooooooooooooooooooooooooong-configmap\":\"eee\",\"secret1-secret\":\"ccc\",\"secret2-secret\":\"ddd\"}",
		}, annotations)
	})
}
