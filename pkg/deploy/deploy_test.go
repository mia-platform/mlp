// Copyright 2020 Mia srl
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package deploy

import (
	"context"

	"testing"
	"time"

	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestEnsureNamespaceExistance(t *testing.T) {
	t.Run("Ensure Namespace existance", func(t *testing.T) {
		namespaceName := "foo"
		scheme := runtime.NewScheme()
		_ = corev1.AddToScheme(scheme)
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme)
		clients := k8sClients{dynamic: dynamicClient}

		err := ensureNamespaceExistence(&clients, namespaceName)
		require.Nil(t, err, "No errors when namespace does not exists")

		_, err = dynamicClient.Resource(gvrNamespaces).
			Get(context.Background(), namespaceName, metav1.GetOptions{})
		require.Nil(t, err)

		err = ensureNamespaceExistence(&clients, namespaceName)
		require.Nil(t, err, "No errors when namespace already exists")
		_, err = dynamicClient.Resource(gvrNamespaces).
			Get(context.Background(), namespaceName, metav1.GetOptions{})
		require.Nil(t, err)
	})
}

func TestCheckIfCreateJob(t *testing.T) {
	cronjob, err := resourceutil.NewResources("testdata/cronjob-test.cronjob.yml", "default")
	require.Nil(t, err)
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	t.Run("without last-applied", func(t *testing.T) {
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme)
		err := checkIfCreateJob(dynamicClient, &cronjob[0].Object, cronjob[0])
		require.Nil(t, err)
	})
	t.Run("same last-applied", func(t *testing.T) {
		cronjob[0].Object.SetAnnotations(map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"batch/v1beta1\",\"kind\":\"CronJob\",\"metadata\":{\"annotations\":{\"mia-platform.eu/autocreate\":\"true\"},\"name\":\"hello\",\"namespace\":\"default\"},\"spec\":{\"jobTemplate\":{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"args\":[\"/bin/sh\",\"-c\",\"date; sleep 120\"],\"image\":\"busybox\",\"name\":\"hello\"}],\"restartPolicy\":\"OnFailure\"}}}},\"schedule\":\"*/5 * * * *\"}}\n",
			"mia-platform.eu/autocreate":                       "true",
		})
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme)
		err = checkIfCreateJob(dynamicClient, &cronjob[0].Object, cronjob[0])
		require.Nil(t, err)
	})
	t.Run("different last-applied", func(t *testing.T) {
		obj := cronjob[0].Object.DeepCopy()
		obj.SetAnnotations(map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"batch/v1beta1\",\"kind\":\"CronJob\",\"metadata\":{\"annotations\":{\"mia-platform.eu/autocreate\":\"true\"},\"name\":\"hello\",\"namespace\":\"default\"},\"spec\":{\"jobTemplate\":{\"spec\":{\"template\":{\"spec\":{\"containers\":[{\"args\":[\"/bin/sh\",\"-c\",\"date; sleep 2\"],\"image\":\"busybox\",\"name\":\"hello\"}],\"restartPolicy\":\"OnFailure\"}}}},\"schedule\":\"*/5 * * * *\"}}\n",
		})
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme)
		err = checkIfCreateJob(dynamicClient, obj, cronjob[0])
		require.Nil(t, err)
		list, err := dynamicClient.Resource(gvrJobs).
			Namespace("default").List(context.Background(), metav1.ListOptions{})
		require.Nil(t, err)
		require.Equal(t, 1, len(list.Items))
	})

}

func TestCronJobAutoCreate(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	testcases := []struct {
		description string
		setup       func(obj *unstructured.Unstructured)
		expected    int
	}{
		{
			description: "autocreate true",
			expected:    1,
			setup: func(obj *unstructured.Unstructured) {
				obj.SetAnnotations(map[string]string{
					"mia-platform.eu/autocreate": "true",
				})
			},
		},
		{
			description: "autocreate false",
			expected:    0,
			setup: func(obj *unstructured.Unstructured) {
				obj.SetAnnotations(map[string]string{
					"mia-platform.eu/autocreate": "false",
				})
			},
		},
		{
			description: "no annotation",
			expected:    0,
			setup: func(obj *unstructured.Unstructured) {
				obj.SetAnnotations(map[string]string{})
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			cronjob, err := resourceutil.NewResources("testdata/cronjob-test.cronjob.yml", "default")
			require.Nil(t, err)
			tt.setup(&cronjob[0].Object)
			dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme)
			err = cronJobAutoCreate(dynamicClient, &cronjob[0].Object)
			require.Nil(t, err)
			list, err := dynamicClient.Resource(gvrJobs).
				Namespace("default").List(context.Background(), metav1.ListOptions{})
			require.Nil(t, err)
			require.Equal(t, tt.expected, len(list.Items))
		})
	}
}

func TestCreateJobFromCronJob(t *testing.T) {
	cron, err := resourceutil.NewResources("testdata/cronjob-test.cronjob.yml", "default")
	require.Nil(t, err)
	expected := map[string]interface{}{"apiVersion": "batch/v1", "kind": "Job", "metadata": map[string]interface{}{"annotations": map[string]interface{}{"cronjob.kubernetes.io/instantiate": "manual"}, "creationTimestamp": interface{}(nil), "generateName": "hello-", "namespace": "default"}, "spec": map[string]interface{}{"template": map[string]interface{}{"metadata": map[string]interface{}{"creationTimestamp": interface{}(nil)}, "spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"args": []interface{}{"/bin/sh", "-c", "date; sleep 120"}, "image": "busybox", "name": "hello", "resources": map[string]interface{}{}}}, "restartPolicy": "OnFailure"}}}, "status": map[string]interface{}{}}

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme)

	jobName, err := createJobFromCronjob(dynamicClient, &cron[0].Object)
	require.Nil(t, err)
	actual, err := dynamicClient.Resource(gvrJobs).
		Namespace("default").
		Get(context.Background(), jobName, metav1.GetOptions{})
	require.Nil(t, err)
	require.Equal(t, expected, actual.Object)
}

func TestCreatePatch(t *testing.T) {
	t.Run("Pass the same object should produce empty patch", func(t *testing.T) {
		deployment, err := resourceutil.NewResources("testdata/test-deployment.yaml", "default")
		require.Nil(t, err)

		deployment[0].Object.SetAnnotations(map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":null,\"labels\":{\"app\":\"test-deployment\"},\"name\":\"test-deployment\",\"namespace\":\"default\"},\"spec\":{\"replicas\":1,\"selector\":{\"matchLabels\":{\"app\":\"test-deployment\"}},\"strategy\":{},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"app\":\"test-deployment\"}},\"spec\":{\"containers\":[{\"image\":\"nginx\",\"name\":\"nginx\",\"resources\":{}}]}}},\"status\":{}}\n"},
		)

		patch, patchType, err := createPatch(deployment[0].Object, deployment[0])
		require.Equal(t, "{}", string(patch), "patch should be empty")
		require.Equal(t, patchType, types.StrategicMergePatchType)
		require.Nil(t, err)
	})

	t.Run("change replicas", func(t *testing.T) {
		deployment, err := resourceutil.NewResources("testdata/test-deployment.yaml", "default")
		require.Nil(t, err)

		deployment[0].Object.SetAnnotations(map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": "{\"apiVersion\":\"apps/v1\",\"kind\":\"Deployment\",\"metadata\":{\"annotations\":{},\"creationTimestamp\":null,\"labels\":{\"app\":\"test-deployment\"},\"name\":\"test-deployment\",\"namespace\":\"default\"},\"spec\":{\"replicas\":1,\"selector\":{\"matchLabels\":{\"app\":\"test-deployment\"}},\"strategy\":{},\"template\":{\"metadata\":{\"creationTimestamp\":null,\"labels\":{\"app\":\"test-deployment\"}},\"spec\":{\"containers\":[{\"image\":\"nginx\",\"name\":\"nginx\",\"resources\":{}}]}}},\"status\":{}}\n"},
		)

		oldDeploy := deployment[0].Object.DeepCopy()
		err = unstructured.SetNestedField(oldDeploy.Object, "2",
			"spec", "replicas")
		require.Nil(t, err)
		t.Logf("oldobj: %s\n", oldDeploy.Object)
		patch, patchType, err := createPatch(*oldDeploy, deployment[0])
		require.Equal(t, "{\"spec\":{\"replicas\":1}}", string(patch), "patch should contain the new resource name")
		require.Equal(t, patchType, types.StrategicMergePatchType)
		require.Nil(t, err)
	})
}

func TestUpdateResourceSecret(t *testing.T) {
	expected := corev1.Secret{
		Data: map[string][]byte{"resources": []byte(`{"CronJob":{"kind":{"Group":"batch","Version":"v1beta1","Kind":"CronJob"},"resources":["bar"]}}`)},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceSecretName,
			Namespace: "foo",
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	resources := map[string]*ResourceList{
		"CronJob": {
			Gvk:       &schema.GroupVersionKind{Group: "batch", Version: "v1beta1", Kind: "CronJob"},
			Resources: []string{"bar"},
		},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	t.Run("Create resource-deployed secret for the first time", func(t *testing.T) {
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme)
		err := updateResourceSecret(dynamicClient, "foo", resources)
		require.Nil(t, err)
		var actual corev1.Secret
		expUnstr, err := dynamicClient.Resource(gvrSecrets).
			Namespace("foo").
			Get(context.Background(), resourceSecretName, metav1.GetOptions{})
		require.Nil(t, err)
		runtime.DefaultUnstructuredConverter.FromUnstructured(expUnstr.Object, &actual)
		require.Equal(t, string(expected.Data["resources"]), string(actual.Data["resources"]))
	})
	t.Run("Update resource-deployed", func(t *testing.T) {
		existingSecret := &corev1.Secret{
			Data: map[string][]byte{"resources": []byte(`{"CronJob":{"kind":{"Group":"batch","Version":"v1beta1","Kind":"CronJob"},"resources":["foo", "sss"]}}`)},
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceSecretName,
				Namespace: "foo",
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme, existingSecret)

		err := updateResourceSecret(dynamicClient, "foo", resources)
		require.Nil(t, err)
		var actual corev1.Secret
		expUnstr, err := dynamicClient.Resource(gvrSecrets).
			Namespace("foo").
			Get(context.Background(), resourceSecretName, metav1.GetOptions{})
		require.Nil(t, err)
		runtime.DefaultUnstructuredConverter.FromUnstructured(expUnstr.Object, &actual)
		require.Equal(t, string(expected.Data["resources"]), string(actual.Data["resources"]))
	})
}

func TestPrune(t *testing.T) {
	deleteList := &ResourceList{
		Gvk:       &schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"},
		Resources: []string{"foo"},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	client := fakeclientset.NewSimpleClientset()
	fakeDiscovery, ok := client.Discovery().(*fakediscovery.FakeDiscovery)
	if !ok {
		t.Fatalf("couldn't convert Discovery() to *FakeDiscovery")
	}
	fakeDiscovery.Fake.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{
					Kind: "Secret",
					Name: "secrets",
				},
			},
		},
		{
			GroupVersion: "batch/v1beta1",
			APIResources: []metav1.APIResource{
				{
					Kind: "CronJob",
					Name: "cronjobs",
				},
			},
		},
	}

	t.Run("Prune resource containing label", func(t *testing.T) {
		toPrune := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
				Labels: map[string]string{
					resourceutil.ManagedByLabel: resourceutil.ManagedByMia,
				},
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme, toPrune)

		clients := k8sClients{
			dynamic:   dynamicClient,
			discovery: fakeDiscovery,
		}
		obj, err := dynamicClient.Resource(gvrSecrets).Namespace("bar").
			Get(context.Background(), "foo", metav1.GetOptions{})
		t.Logf("err: %s", obj)
		require.Nil(t, err)

		err = prune(&clients, "bar", deleteList)
		require.Nil(t, err)

		_, err = dynamicClient.Resource(gvrSecrets).Namespace("bar").
			Get(context.Background(), "foo", metav1.GetOptions{})
		require.Equal(t, true, apierrors.IsNotFound(err))
	})

	t.Run("Don't prune resource without label", func(t *testing.T) {
		toPrune := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme, toPrune)

		clients := k8sClients{
			dynamic:   dynamicClient,
			discovery: fakeDiscovery,
		}

		err := prune(&clients, "bar", deleteList)
		require.Nil(t, err)
		_, err = dynamicClient.Resource(gvrSecrets).Namespace("bar").
			Get(context.Background(), "foo", metav1.GetOptions{})
		require.Equal(t, false, apierrors.IsNotFound(err))
	})

	t.Run("Skip non existing resource", func(t *testing.T) {
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme)
		clients := k8sClients{
			dynamic:   dynamicClient,
			discovery: fakeDiscovery,
		}
		err := prune(&clients, "bar", deleteList)
		require.Nil(t, err)
		_, err = dynamicClient.Resource(gvrSecrets).Namespace("bar").
			Get(context.Background(), "foo", metav1.GetOptions{})
		require.Equal(t, true, apierrors.IsNotFound(err))
	})
}

func TestEnsureDeployAll(t *testing.T) {

	mockTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	expectedCheckSum := "6ab733c74e26e73bca78aa9c4c9db62664f339d9eefac51dd503c9ff0cf0c329"

	t.Run("Add deployment annotation", func(t *testing.T) {
		deployment, err := resourceutil.NewResources("testdata/test-deployment.yaml", "default")
		require.Nil(t, err)
		err = ensureDeployAll(&deployment[0], mockTime)
		require.Nil(t, err)

		var dep appsv1.Deployment
		err = runtime.DefaultUnstructuredConverter.
			FromUnstructured(deployment[0].Object.Object, &dep)
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}, dep.Spec.Template.ObjectMeta.Annotations)
	})

	t.Run("Add cronJob annotation", func(t *testing.T) {
		cronJob, err := resourceutil.NewResources("testdata/cronjob-test.cronjob.yml", "default")
		require.Nil(t, err)

		err = ensureDeployAll(&cronJob[0], mockTime)
		require.Nil(t, err)
		var cronj batchv1beta1.CronJob
		err = runtime.DefaultUnstructuredConverter.
			FromUnstructured(cronJob[0].Object.Object, &cronj)
		require.Nil(t, err)
		require.Equal(t, map[string]string{resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum}, cronj.Spec.JobTemplate.Spec.Template.ObjectMeta.Annotations)
	})
	t.Run("Keep existing annotations", func(t *testing.T) {
		// testing only deployment because annotation accessing method is the same
		deployment, err := resourceutil.NewResources("testdata/test-deployment.yaml", "default")
		require.Nil(t, err)
		unstructured.SetNestedStringMap(deployment[0].Object.Object, map[string]string{
			"existing-key": "value1",
		},
			"spec", "template", "metadata", "annotations")
		err = ensureDeployAll(&deployment[0], mockTime)
		require.Nil(t, err)
		var dep appsv1.Deployment
		err = runtime.DefaultUnstructuredConverter.
			FromUnstructured(deployment[0].Object.Object, &dep)
		require.Nil(t, err)
		require.Equal(t, map[string]string{
			resourceutil.GetMiaAnnotation(deployChecksum): expectedCheckSum,
			"existing-key": "value1",
		}, dep.Spec.Template.ObjectMeta.Annotations)
	})
}

func TestEnsureSmartDeploy(t *testing.T) {
	expectedCheckSum := "6ab733c74e26e73bca78aa9c4c9db62664f339d9eefac51dd503c9ff0cf0c329"

	t.Run("Add deployment deploy/checksum annotation", func(t *testing.T) {
		targetObject, err := resourceutil.NewResources("testdata/test-deployment.yaml", "default")
		require.Nil(t, err)
		currentObj := targetObject[0].Object.DeepCopy()
		unstructured.SetNestedStringMap(currentObj.Object, map[string]string{
			"mia-platform.eu/deploy-checksum": expectedCheckSum,
			"test":                            "test",
		}, "spec", "template", "metadata", "annotations")
		t.Logf("targetObj: %s\n", currentObj.Object)
		err = ensureSmartDeploy(currentObj, &targetObject[0])
		require.Nil(t, err)
		targetAnn, _, err := unstructured.NestedStringMap(targetObject[0].Object.Object,
			"spec", "template", "metadata", "annotations")
		require.Nil(t, err)
		require.Equal(t, targetAnn["mia-platform.eu/deploy-checksum"], expectedCheckSum)
	})

	t.Run("Add deployment without deploy/checksum annotation", func(t *testing.T) {
		targetObject, err := resourceutil.NewResources("testdata/test-deployment.yaml", "default")
		require.Nil(t, err)
		currentObj := targetObject[0].Object.DeepCopy()
		err =
			unstructured.SetNestedStringMap(targetObject[0].Object.Object, map[string]string{
				"test": "test",
			}, "spec", "template", "annotations")
		require.Nil(t, err)

		err = ensureSmartDeploy(currentObj, &targetObject[0])
		require.Nil(t, err)

		targetAnn, _, err := unstructured.NestedStringMap(targetObject[0].Object.Object,
			"spec", "template", "annotations")
		require.Nil(t, err)
		require.Equal(t, "test", targetAnn["test"])
	})

	t.Run("Add cronjob deploy/checksum annotation", func(t *testing.T) {
		targetObject, err := resourceutil.NewResources("testdata/cronjob-test.cronjob.yml", "default")
		require.Nil(t, err)
		currentObj := targetObject[0].Object.DeepCopy()
		unstructured.SetNestedStringMap(currentObj.Object, map[string]string{
			"mia-platform.eu/deploy-checksum": expectedCheckSum,
			"test":                            "test",
		}, "spec", "jobTemplate", "spec", "template", "metadata", "annotations")
		t.Logf("targetObj: %s\n", currentObj.Object)
		err = ensureSmartDeploy(currentObj, &targetObject[0])

		require.Nil(t, err)
		targetAnn, _, err := unstructured.NestedStringMap(targetObject[0].Object.Object,
			"spec", "jobTemplate", "spec", "template", "metadata", "annotations")
		require.Nil(t, err)
		require.Equal(t, targetAnn["mia-platform.eu/deploy-checksum"], expectedCheckSum)
	})
}

func TestInsertDependencies(t *testing.T) {
	var configMapMap = map[string]string{"configMap1": "aaa", "configMap2": "bbb", "configMapLongLoongLoooooooooooooooooooooooooooooooooooooooooooooong": "eee"}
	var secretMap = map[string]string{"secret1": "ccc", "secret2": "ddd"}
	testVolumes := []corev1.Volume{
		{
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "secret1",
				},
			},
		},
		{
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: "secret2",
				},
			},
		},
		{
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "configMap1",
					},
				},
			},
		},
		{
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "configMap2",
					},
				},
			},
		},
		{
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "configMapLongLoongLoooooooooooooooooooooooooooooooooooooooooooooong",
					},
				},
			},
		},
	}

	t.Run("Test Deployment", func(t *testing.T) {
		deploymentRes, err := resourceutil.NewResources("testdata/test-deployment.yaml", "default")
		require.Nil(t, err)
		var deployment appsv1.Deployment
		err = runtime.DefaultUnstructuredConverter.
			FromUnstructured(deploymentRes[0].Object.Object, &deployment)
		require.Nil(t, err)

		podSpec := resourceutil.GetPodSpec(&testVolumes, nil)
		deployment.Spec.Template.Spec = podSpec

		unstr, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&deployment)
		require.Nil(t, err)
		deploymentRes[0].Object.Object = unstr

		err = insertDependencies(&deploymentRes[0], configMapMap, secretMap)
		require.Nil(t, err)
		expected := "{\"configMap1-configmap\":\"aaa\",\"configMap2-configmap\":\"bbb\",\"configMapLongLoongLoooooooooooooooooooooooooooooooooooooooooooooong-configmap\":\"eee\",\"secret1-secret\":\"ccc\",\"secret2-secret\":\"ddd\"}"
		currentAnnotations, found, err := unstructured.NestedStringMap(deploymentRes[0].Object.Object,
			"spec", "template", "metadata", "annotations")

		require.Nil(t, err)
		require.True(t, found)
		require.Equal(t, expected, currentAnnotations[resourceutil.GetMiaAnnotation(dependenciesChecksum)])
	})

	t.Run("Test CronJob", func(t *testing.T) {
		cronJobRes, err := resourceutil.NewResources("testdata/cronjob-test.cronjob.yml", "default")
		require.Nil(t, err)
		var cronJob batchv1beta1.CronJob
		err = runtime.DefaultUnstructuredConverter.
			FromUnstructured(cronJobRes[0].Object.Object, &cronJob)
		require.Nil(t, err)
		podSpec := resourceutil.GetPodSpec(&testVolumes, nil)
		cronJob.Spec.JobTemplate.Spec.Template.Spec = podSpec

		unstr, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&cronJob)
		require.Nil(t, err)
		cronJobRes[0].Object.Object = unstr

		err = insertDependencies(&cronJobRes[0], configMapMap, secretMap)
		require.Nil(t, err)
		expected := "{\"configMap1-configmap\":\"aaa\",\"configMap2-configmap\":\"bbb\",\"configMapLongLoongLoooooooooooooooooooooooooooooooooooooooooooooong-configmap\":\"eee\",\"secret1-secret\":\"ccc\",\"secret2-secret\":\"ddd\"}"
		currentAnnotations, found, err := unstructured.NestedStringMap(cronJobRes[0].Object.Object,
			"spec", "jobTemplate", "spec", "template", "metadata", "annotations")
		require.Nil(t, err)
		require.True(t, found)
		require.Equal(t, expected, currentAnnotations[resourceutil.GetMiaAnnotation(dependenciesChecksum)])
	})
}
