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
	"errors"
	"testing"
	"time"

	"github.com/mia-platform/mlp/internal/utils"
	"github.com/mia-platform/mlp/pkg/resourceutil"
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
	"k8s.io/apimachinery/pkg/watch"
	discoveryFake "k8s.io/client-go/discovery/fake"
	dynamicFake "k8s.io/client-go/dynamic/fake"
	clientsetFake "k8s.io/client-go/kubernetes/fake"
	k8stest "k8s.io/client-go/testing"
)

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

func TestHandleResourceCompletionEvent(t *testing.T) {
	job := resourceutil.Resource{
		GroupVersionKind: &schema.GroupVersionKind{
			Group:   "batch",
			Version: "v1",
			Kind:    "Job",
		},
		Object: unstructured.Unstructured{
			Object: map[string]interface{}{
				"metadata": map[string]string{
					"name": "job-name",
				},
			},
		},
	}

	testCases := []struct {
		desc         string
		startTime    time.Time
		job          resourceutil.Resource
		event        *watch.Event
		isCompleted  bool
		errorRequire func(require.TestingT, interface{}, ...interface{})
	}{
		{
			desc:      "Handles Job resources",
			startTime: time.Now(),
			job: resourceutil.Resource{
				GroupVersionKind: &schema.GroupVersionKind{
					Group:   "batch",
					Version: "v1",
					Kind:    "Job",
				},
			},
			event:        nil,
			isCompleted:  false,
			errorRequire: require.Nil,
		}, {
			desc:      "Does not handle Unknown resources",
			startTime: time.Now(),
			job: resourceutil.Resource{
				GroupVersionKind: &schema.GroupVersionKind{
					Group:   "foo",
					Version: "bar",
					Kind:    "Unknown",
				},
			},
			event:        nil,
			isCompleted:  false,
			errorRequire: require.NotNil,
		}, {
			desc:      "Correctly handles jobs completed after the start time",
			startTime: time.Now(),
			job:       job,
			event: &watch.Event{
				Type: watch.Modified,
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]string{
							"name": "job-name",
						},
						"status": map[string]interface{}{
							"completionTime": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
						},
					},
				},
			},
			isCompleted:  true,
			errorRequire: require.Nil,
		}, {
			desc:      "Correctly handles jobs completed before the start time",
			startTime: time.Now().Add(time.Hour),
			job:       job,
			event: &watch.Event{
				Type: watch.Modified,
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]string{
							"name": "job-name",
						},
						"status": map[string]interface{}{
							"completionTime": time.Now().UTC().Format(time.RFC3339),
						},
					},
				},
			},
			isCompleted:  false,
			errorRequire: require.Nil,
		}, {
			desc:      "Correctly handles incomplete jobs",
			startTime: time.Now().Add(time.Hour),
			job:       job,
			event: &watch.Event{
				Type: watch.Modified,
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]string{
							"name": "job-name",
						},
					},
				},
			},
			isCompleted:  false,
			errorRequire: require.Nil,
		}, {
			desc:      "Matches resources by name",
			startTime: time.Now(),
			job:       job,
			event: &watch.Event{
				Type: watch.Modified,
				Object: &unstructured.Unstructured{
					Object: map[string]interface{}{
						"metadata": map[string]string{
							"name": "other-job-name",
						},
						"status": map[string]interface{}{
							"completionTime": time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
						},
					},
				},
			},
			isCompleted:  false,
			errorRequire: require.Nil,
		},
	}
	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			isCompleted, err := handleResourceCompletionEvent(tC.job, tC.event, tC.startTime)

			require.Exactly(t, tC.isCompleted, isCompleted)
			tC.errorRequire(t, err)
		})
	}
}

func TestWithAwaitableResource(t *testing.T) {
	testCases := []struct {
		desc          string
		resFileName   string
		watcherEvents []unstructured.Unstructured
		errorRequire  func(require.TestingT, interface{}, ...interface{})
	}{
		{
			desc:          "Ignores non annotated resources",
			resFileName:   "testdata/simple-job.yaml",
			watcherEvents: []unstructured.Unstructured{},
			errorRequire:  require.Nil,
		}, {
			desc:        "Awaits annotated resources for completion",
			resFileName: "testdata/awaitable-job.yaml",
			watcherEvents: []unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "batch/v1",
						"kind":       "Job",
						"metadata": map[string]string{
							"name": "awaitable-job",
						},
						"status": map[string]interface{}{
							"completionTime": time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
						},
					},
				},
			},
			errorRequire: require.Nil,
		}, {
			desc:          "Awaits annotated resources for completion",
			resFileName:   "testdata/awaitable-job.yaml",
			watcherEvents: []unstructured.Unstructured{},
			errorRequire:  require.NotNil,
		},
	}

	deployConfig := utils.DeployConfig{}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			clients := createFakeClientsWithJobs(t)
			resources, err := resourceutil.NewResources(tC.resFileName, "default")
			require.Nil(t, err)
			res := resources[0]

			watcher := watch.NewFakeWithChanSize(len(tC.watcherEvents), false)
			clients.dynamic.(*dynamicFake.FakeDynamicClient).Fake.PrependWatchReactor("jobs", k8stest.DefaultWatchReactor(watcher, nil))

			for _, e := range tC.watcherEvents {
				watcher.Modify(&e)
			}

			applyCalled := false
			err = withAwaitableResource(func(c *k8sClients, r resourceutil.Resource, d utils.DeployConfig) error {
				applyCalled = true
				require.Exactly(t, &clients, c)
				require.Exactly(t, res, r)
				require.Exactly(t, deployConfig, d)
				return nil
			})(&clients, res, deployConfig)

			require.True(t, applyCalled)
			tC.errorRequire(t, err)
		})
	}

	t.Run("Forwards inner apply errors", func(t *testing.T) {
		clients := createFakeClientsWithJobs(t)
		res := resourceutil.Resource{
			GroupVersionKind: &schema.GroupVersionKind{
				Group:   "batch",
				Version: "v1",
				Kind:    "Job",
			},
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]string{
						"name": "awaitable-job",
					},
				},
			},
		}
		expectedErr := errors.New("Some apply error")

		actualErr := withAwaitableResource(func(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error {
			return expectedErr
		})(&clients, res, deployConfig)

		require.Exactly(t, expectedErr, actualErr)
	})
}

func TestWithDeletableResource(t *testing.T) {
	resName := "res-name"
	requireResourceExists := func(shouldExist bool) func(t *testing.T, clients *k8sClients, res resourceutil.Resource) {
		return func(t *testing.T, clients *k8sClients, res resourceutil.Resource) {
			gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, *res.GroupVersionKind)
			require.Nil(t, err)
			_, err = clients.dynamic.Resource(gvr).
				Get(context.TODO(), resName, metav1.GetOptions{})

			if shouldExist {
				require.Nil(t, err)
			} else {
				require.NotNil(t, err)
				require.True(t, apierrors.IsNotFound(err))
			}
		}
	}
	testCases := []struct {
		desc        string
		annotations map[string]interface{}
		setup       func(t *testing.T, clients *k8sClients, res resourceutil.Resource)
		requireFn   func(t *testing.T, clients *k8sClients, res resourceutil.Resource)
	}{
		{
			desc: "Deletes annotated resources before apply",
			annotations: map[string]interface{}{
				deleteBeforeApplyAnnotation: "true",
			},
			setup: func(t *testing.T, clients *k8sClients, res resourceutil.Resource) {
				gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, *res.GroupVersionKind)
				require.Nil(t, err)
				_, err = clients.dynamic.Resource(gvr).
					Create(context.TODO(), &res.Object, metav1.CreateOptions{})
				require.Nil(t, err)
			},
			requireFn: requireResourceExists(false),
		}, {
			desc:        "Ignores non annotated resources",
			annotations: map[string]interface{}{},
			setup: func(t *testing.T, clients *k8sClients, res resourceutil.Resource) {
				gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, *res.GroupVersionKind)
				require.Nil(t, err)
				_, err = clients.dynamic.Resource(gvr).
					Create(context.TODO(), &res.Object, metav1.CreateOptions{})
				require.Nil(t, err)
			},
			requireFn: requireResourceExists(true),
		}, {
			desc: "Correctly handles non existing resources",
			annotations: map[string]interface{}{
				deleteBeforeApplyAnnotation: "true",
			},
			setup:     func(t *testing.T, clients *k8sClients, res resourceutil.Resource) {},
			requireFn: requireResourceExists(false),
		},
	}
	deployConfig := utils.DeployConfig{}

	for _, tC := range testCases {
		t.Run(tC.desc, func(t *testing.T) {
			clients := createFakeClientsWithJobs(t)
			res := resourceutil.Resource{
				GroupVersionKind: &schema.GroupVersionKind{
					Group:   "batch",
					Version: "v1",
					Kind:    "Job",
				},
				Object: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "batch/v1",
						"kind":       "Job",
						"metadata": map[string]interface{}{
							"name":        resName,
							"annotations": tC.annotations,
						},
					},
				},
			}

			tC.setup(t, &clients, res)

			applyCalled := false
			err := withDeletableResource(func(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error {
				applyCalled = true
				tC.requireFn(t, clients, res)
				return nil
			})(&clients, res, deployConfig)
			require.True(t, applyCalled)
			require.Nil(t, err)
		})
	}

	t.Run("Forwards inner apply errors", func(t *testing.T) {
		clients := createFakeClientsWithJobs(t)
		res := resourceutil.Resource{
			GroupVersionKind: &schema.GroupVersionKind{
				Group:   "batch",
				Version: "v1",
				Kind:    "Job",
			},
			Object: unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]string{
						"name": "awaitable-job",
					},
				},
			},
		}
		expectedErr := errors.New("Some apply error")

		actualErr := withAwaitableResource(func(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error {
			return expectedErr
		})(&clients, res, deployConfig)

		require.Exactly(t, expectedErr, actualErr)
	})
}

func createFakeClients(t *testing.T, resourcesList []*metav1.APIResourceList) k8sClients {
	t.Helper()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = batchv1.AddToScheme(scheme)

	clients := k8sClients{
		dynamic:   dynamicFake.NewSimpleDynamicClient(scheme),
		discovery: clientsetFake.NewSimpleClientset().Discovery(),
	}

	discovery, ok := clients.discovery.(*discoveryFake.FakeDiscovery)
	require.True(t, ok)
	discovery.Fake.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "batch/v1",
			APIResources: []metav1.APIResource{
				{
					Kind: "Job",
					Name: "jobs",
				},
			},
		},
	}

	return clients
}

func createFakeClientsWithJobs(t *testing.T) k8sClients {
	t.Helper()
	resList := []*metav1.APIResourceList{
		{
			GroupVersion: "batch/v1",
			APIResources: []metav1.APIResource{
				{
					Kind: "Job",
					Name: "jobs",
				},
			},
		},
	}
	return createFakeClients(t, resList)
}
