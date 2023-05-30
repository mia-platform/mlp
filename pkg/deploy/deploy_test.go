// Copyright Mia srl
// SPDX-License-Identifier: Apache-2.0
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

	"github.com/mia-platform/mlp/pkg/resourceutil"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	fakeclientset "k8s.io/client-go/kubernetes/fake"
)

func TestEnsureNamespaceExistence(t *testing.T) {
	t.Run("Ensure Namespace existence", func(t *testing.T) {
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

func TestUpdateResourceSecret(t *testing.T) {
	const namespace = "foo"

	expected := corev1.Secret{
		Data: map[string][]byte{"resources": []byte(`{"CronJob":{"gvk":{"Group":"batch","Version":"v1","Kind":"CronJob"},"resources":["bar"]}}`)},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceSecretName,
			Namespace: "foo",
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	resources := map[string]*ResourceList{
		"CronJob": {
			Gvk:       &schema.GroupVersionKind{Group: "batch", Version: "v1", Kind: "CronJob"},
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
			Data: map[string][]byte{"resources": []byte(`{"CronJob":{"kind":{"Group":"batch","Version":"v1","Kind":"CronJob"},"resources":["foo", "sss"]}}`)},
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

	t.Run("Update resource-deployed when empty namespace on cluster", func(t *testing.T) {
		existingSecret := &corev1.Secret{
			Data: map[string][]byte{"resources": []byte(`{}`)},
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceSecretName,
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme, existingSecret)

		err := updateResourceSecret(dynamicClient, namespace, resources)
		require.Nil(t, err)
		var actual corev1.Secret
		expUnstr, err := dynamicClient.Resource(gvrSecrets).
			Namespace(namespace).
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
			GroupVersion: "batch/v1",
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
		var cronJob batchv1.CronJob
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

func TestCleanup(t *testing.T) {
	const namespace = "foo"
	expected := corev1.Secret{
		Data: map[string][]byte{"resources": []byte(`{"Deployment":{"gvk":{"Group":"apps","Version":"v1","Kind":"Deployment"},"resources":["test-deployment"]}}`)},
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceSecretName,
			Namespace: "foo",
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	deployObject := unstructured.Unstructured{}
	deployObject.SetName("test-deployment")

	resources := []resourceutil.Resource{
		{
			Filepath:         "./testdata/test-deployment.yaml",
			GroupVersionKind: &schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			Object:           deployObject,
		},
	}
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	t.Run("Update resource-deployed when empty namespace on cluster", func(t *testing.T) {
		existingSecret := &corev1.Secret{
			Data: map[string][]byte{"resources": []byte(`{}`)},
			ObjectMeta: metav1.ObjectMeta{
				Name:      resourceSecretName,
				Namespace: namespace,
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		dynamicClient := dynamicFake.NewSimpleDynamicClient(scheme, existingSecret)
		client := &k8sClients{dynamic: dynamicClient}

		err := cleanup(client, namespace, resources)
		require.Nil(t, err)
		var actual corev1.Secret
		expUnstr, err := dynamicClient.Resource(gvrSecrets).
			Namespace(namespace).
			Get(context.Background(), resourceSecretName, metav1.GetOptions{})
		require.Nil(t, err)
		runtime.DefaultUnstructuredConverter.FromUnstructured(expUnstr.Object, &actual)
		require.Equal(t, string(expected.Data["resources"]), string(actual.Data["resources"]))
	})
}
