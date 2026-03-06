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
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// newTestJob creates an unstructured Job resource for testing with the given name and optional
// pre-deploy annotation value. If annotationValue is empty, no annotation is added.
func newTestJob(name, annotationValue string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "test",
								"image": "busybox",
							},
						},
						"restartPolicy": "Never",
					},
				},
			},
		},
	}

	if annotationValue != "" {
		obj.SetAnnotations(map[string]string{
			preDeployAnnotation: annotationValue,
		})
	}

	return obj
}

// newOptionalTestJob creates a pre-deploy Job with the deploy-optional annotation set to "true".
func newOptionalTestJob(name string) *unstructured.Unstructured {
	job := newTestJob(name, "pre-deploy")
	annotations := job.GetAnnotations()
	annotations[deployOptionalAnnotation] = "true"
	job.SetAnnotations(annotations)
	return job
}

func TestFilterPreDeployJobs(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		resources         []*unstructured.Unstructured
		annotationValue   string
		expectedJobs      int
		expectedRemaining int
	}{
		"no resources": {
			resources:         nil,
			annotationValue:   "pre-deploy",
			expectedJobs:      0,
			expectedRemaining: 0,
		},
		"no matching jobs": {
			resources: []*unstructured.Unstructured{
				newTestJob("job1", ""),
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "cm1"},
					},
				},
			},
			annotationValue:   "pre-deploy",
			expectedJobs:      0,
			expectedRemaining: 2,
		},
		"matching jobs separated from remaining": {
			resources: []*unstructured.Unstructured{
				newTestJob("pre-job", "pre-deploy"),
				newTestJob("normal-job", ""),
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata":   map[string]interface{}{"name": "deploy1"},
					},
				},
			},
			annotationValue:   "pre-deploy",
			expectedJobs:      1,
			expectedRemaining: 2,
		},
		"non-job resource with annotation not filtered": {
			resources: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":        "cm-with-annotation",
							"annotations": map[string]interface{}{preDeployAnnotation: "pre-deploy"},
						},
					},
				},
			},
			annotationValue:   "pre-deploy",
			expectedJobs:      0,
			expectedRemaining: 1,
		},
		"job with different annotation value not filtered": {
			resources: []*unstructured.Unstructured{
				newTestJob("job-once", "once"),
			},
			annotationValue:   "pre-deploy",
			expectedJobs:      0,
			expectedRemaining: 1,
		},
		"multiple matching jobs": {
			resources: []*unstructured.Unstructured{
				newTestJob("pre-job-1", "pre-deploy"),
				newTestJob("pre-job-2", "pre-deploy"),
				newTestJob("normal-job", ""),
			},
			annotationValue:   "pre-deploy",
			expectedJobs:      2,
			expectedRemaining: 1,
		},
		"custom annotation value": {
			resources: []*unstructured.Unstructured{
				newTestJob("custom-job", "custom-value"),
				newTestJob("pre-job", "pre-deploy"),
			},
			annotationValue:   "custom-value",
			expectedJobs:      1,
			expectedRemaining: 1,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			jobs, remaining := FilterPreDeployJobs(test.resources, test.annotationValue)
			assert.Len(t, jobs, test.expectedJobs)
			assert.Len(t, remaining, test.expectedRemaining)
		})
	}
}

func TestStripAnnotatedJobs(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		resources         []*unstructured.Unstructured
		expectedRemaining int
	}{
		"no resources": {
			resources:         nil,
			expectedRemaining: 0,
		},
		"no annotated jobs": {
			resources: []*unstructured.Unstructured{
				newTestJob("normal-job", ""),
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata":   map[string]interface{}{"name": "cm1"},
					},
				},
			},
			expectedRemaining: 2,
		},
		"strips jobs with any annotation value": {
			resources: []*unstructured.Unstructured{
				newTestJob("pre-job", "pre-deploy"),
				newTestJob("custom-job", "custom-value"),
				newTestJob("normal-job", ""),
				{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Deployment",
						"metadata":   map[string]interface{}{"name": "deploy1"},
					},
				},
			},
			expectedRemaining: 2,
		},
		"non-job resource with annotation is kept": {
			resources: []*unstructured.Unstructured{
				{
					Object: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name":        "cm-annotated",
							"annotations": map[string]interface{}{preDeployAnnotation: "pre-deploy"},
						},
					},
				},
			},
			expectedRemaining: 1,
		},
		"all annotated jobs stripped": {
			resources: []*unstructured.Unstructured{
				newTestJob("job-1", "pre-deploy"),
				newTestJob("job-2", "other"),
			},
			expectedRemaining: 0,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			remaining := StripAnnotatedJobs(test.resources)
			assert.Len(t, remaining, test.expectedRemaining)
		})
	}
}

func TestPreDeployJobRunnerRun(t *testing.T) {
	t.Parallel()

	namespace := "test-ns"

	tests := map[string]struct {
		jobs          []*unstructured.Unstructured
		setupReactor  func(*kubefake.Clientset)
		dryRun        bool
		maxRetries    int
		expectedError string
		expectedOut   string
	}{
		"no jobs returns nil": {
			jobs:       nil,
			maxRetries: 3,
		},
		"dry run skips execution": {
			jobs:        []*unstructured.Unstructured{newTestJob("pre-job", "pre-deploy")},
			dryRun:      true,
			maxRetries:  3,
			expectedOut: "would be executed (dry-run)",
		},
		"successful job completion": {
			jobs: []*unstructured.Unstructured{newTestJob("pre-job", "pre-deploy")},
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      action.(k8stesting.GetAction).GetName(),
							Namespace: namespace,
						},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{
									Type:   batchv1.JobComplete,
									Status: corev1.ConditionTrue,
								},
							},
						},
					}, nil
				})
			},
			maxRetries:  3,
			expectedOut: "completed successfully",
		},
		"all jobs fail returns error": {
			jobs: []*unstructured.Unstructured{newTestJob("fail-job", "pre-deploy")},
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      action.(k8stesting.GetAction).GetName(),
							Namespace: namespace,
						},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{
									Type:    batchv1.JobFailed,
									Status:  corev1.ConditionTrue,
									Message: "BackoffLimitExceeded",
								},
							},
						},
					}, nil
				})
			},
			maxRetries:    1,
			expectedError: "pre-deploy job \"fail-job\" failed after 1 attempts:",
		},
		"partial success stops at first failure": {
			jobs: []*unstructured.Unstructured{
				newTestJob("success-job", "pre-deploy"),
				newTestJob("fail-job", "pre-deploy"),
			},
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					getAction := action.(k8stesting.GetAction)
					name := getAction.GetName()
					if name == "success-job" {
						return true, &batchv1.Job{
							ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
							Status: batchv1.JobStatus{
								Conditions: []batchv1.JobCondition{
									{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
								},
							},
						}, nil
					}
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "failed"},
							},
						},
					}, nil
				})
			},
			maxRetries:    0,
			expectedOut:   "completed successfully",
			expectedError: "pre-deploy job \"fail-job\" failed after 0 attempts:",
		},
		"multiple jobs stops at first failure": {
			jobs: []*unstructured.Unstructured{
				newTestJob("fail-job-1", "pre-deploy"),
				newTestJob("fail-job-2", "pre-deploy"),
			},
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      action.(k8stesting.GetAction).GetName(),
							Namespace: namespace,
						},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "error"},
							},
						},
					}, nil
				})
			},
			maxRetries:    0,
			expectedError: "pre-deploy job \"fail-job-1\" failed after 0 attempts:",
		},
		"optional job failure does not block deploy": {
			jobs: []*unstructured.Unstructured{newOptionalTestJob("optional-job")},
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      action.(k8stesting.GetAction).GetName(),
							Namespace: namespace,
						},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "migration failed"},
							},
						},
					}, nil
				})
			},
			maxRetries:  0,
			expectedOut: "optional pre-deploy job",
		},
		"optional job failure with mandatory success": {
			jobs: []*unstructured.Unstructured{
				newTestJob("mandatory-job", "pre-deploy"),
				newOptionalTestJob("optional-job"),
			},
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					name := action.(k8stesting.GetAction).GetName()
					if name == "mandatory-job" {
						return true, &batchv1.Job{
							ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
							Status: batchv1.JobStatus{
								Conditions: []batchv1.JobCondition{
									{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
								},
							},
						}, nil
					}
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "optional failed"},
							},
						},
					}, nil
				})
			},
			maxRetries:  0,
			expectedOut: "completed successfully",
		},
		"mandatory job fails while optional succeeds": {
			jobs: []*unstructured.Unstructured{
				newTestJob("mandatory-job", "pre-deploy"),
				newOptionalTestJob("optional-job"),
			},
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					name := action.(k8stesting.GetAction).GetName()
					if name == "optional-job" {
						return true, &batchv1.Job{
							ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
							Status: batchv1.JobStatus{
								Conditions: []batchv1.JobCondition{
									{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
								},
							},
						}, nil
					}
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "mandatory failed"},
							},
						},
					}, nil
				})
			},
			maxRetries:    0,
			expectedError: "pre-deploy job \"mandatory-job\" failed after 0 attempts:",
		},
		"all optional jobs fail": {
			jobs: []*unstructured.Unstructured{
				newOptionalTestJob("optional-job-1"),
				newOptionalTestJob("optional-job-2"),
			},
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{
							Name:      action.(k8stesting.GetAction).GetName(),
							Namespace: namespace,
						},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "failed"},
							},
						},
					}, nil
				})
			},
			maxRetries:  0,
			expectedOut: "optional pre-deploy job",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fakeClientset := kubefake.NewSimpleClientset()
			if test.setupReactor != nil {
				test.setupReactor(fakeClientset)
			}

			writer := new(strings.Builder)
			runner := NewPreDeployJobRunner(fakeClientset, namespace, test.maxRetries, 5*time.Second, writer, test.dryRun)
			runner.pollInterval = 10 * time.Millisecond

			err := runner.Run(t.Context(), test.jobs)

			switch len(test.expectedError) {
			case 0:
				require.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}

			if test.expectedOut != "" {
				assert.Contains(t, writer.String(), test.expectedOut)
			}

			t.Log(writer.String())
		})
	}
}

func TestRunJobWithRetries(t *testing.T) {
	t.Parallel()

	namespace := "test-ns"

	tests := map[string]struct {
		maxRetries    int
		setupReactor  func(*kubefake.Clientset)
		expectedError string
	}{
		"success on first attempt": {
			maxRetries: 3,
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
							},
						},
					}, nil
				})
			},
		},
		"success after retry": {
			maxRetries: 3,
			setupReactor: func(cs *kubefake.Clientset) {
				callCount := 0
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					callCount++
					if callCount <= 1 {
						return true, &batchv1.Job{
							ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: namespace},
							Status: batchv1.JobStatus{
								Conditions: []batchv1.JobCondition{
									{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "failed"},
								},
							},
						}, nil
					}
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
							},
						},
					}, nil
				})
			},
		},
		"all retries exhausted": {
			maxRetries: 2,
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "BackoffLimitExceeded"},
							},
						},
					}, nil
				})
			},
			expectedError: `job "test-job" failed: BackoffLimitExceeded`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fakeClientset := kubefake.NewSimpleClientset()
			test.setupReactor(fakeClientset)

			writer := new(strings.Builder)
			runner := NewPreDeployJobRunner(fakeClientset, namespace, test.maxRetries, 5*time.Second, writer, false)
			runner.pollInterval = 10 * time.Millisecond

			job := newTestJob("test-job", "pre-deploy")
			err := runner.runJobWithRetries(t.Context(), job)

			switch len(test.expectedError) {
			case 0:
				require.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}

			t.Log(writer.String())
		})
	}
}

func TestWaitForJobCompletion(t *testing.T) {
	t.Parallel()

	namespace := "test-ns"

	tests := map[string]struct {
		setupReactor  func(*kubefake.Clientset)
		timeout       time.Duration
		expectedError string
	}{
		"job completes successfully": {
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
							},
						},
					}, nil
				})
			},
			timeout: 5 * time.Second,
		},
		"job fails": {
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobFailed, Status: corev1.ConditionTrue, Message: "container crashed"},
							},
						},
					}, nil
				})
			},
			timeout:       5 * time.Second,
			expectedError: `job "test-job" failed: container crashed`,
		},
		"job times out": {
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: namespace},
					}, nil
				})
			},
			timeout:       100 * time.Millisecond,
			expectedError: `job "test-job" timed out`,
		},
		"get job returns error": {
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("connection refused")
				})
			},
			timeout:       5 * time.Second,
			expectedError: `failed to get job "test-job" status`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fakeClientset := kubefake.NewSimpleClientset()
			test.setupReactor(fakeClientset)

			writer := new(strings.Builder)
			runner := NewPreDeployJobRunner(fakeClientset, namespace, 3, test.timeout, writer, false)
			runner.pollInterval = 10 * time.Millisecond

			err := runner.waitForJobCompletion(t.Context(), "test-job")

			switch len(test.expectedError) {
			case 0:
				require.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
		})
	}
}

func TestCreateAndWaitForJob(t *testing.T) {
	t.Parallel()

	namespace := "test-ns"

	tests := map[string]struct {
		job           *unstructured.Unstructured
		setupReactor  func(*kubefake.Clientset)
		expectedError string
	}{
		"successful creation and completion": {
			job: newTestJob("test-job", "pre-deploy"),
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, &batchv1.Job{
						ObjectMeta: metav1.ObjectMeta{Name: "test-job", Namespace: namespace},
						Status: batchv1.JobStatus{
							Conditions: []batchv1.JobCondition{
								{Type: batchv1.JobComplete, Status: corev1.ConditionTrue},
							},
						},
					}, nil
				})
			},
		},
		"creation failure": {
			job: newTestJob("test-job", "pre-deploy"),
			setupReactor: func(cs *kubefake.Clientset) {
				cs.PrependReactor("create", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("forbidden")
				})
			},
			expectedError: `failed to create job "test-job"`,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			fakeClientset := kubefake.NewSimpleClientset()
			test.setupReactor(fakeClientset)

			writer := new(strings.Builder)
			runner := NewPreDeployJobRunner(fakeClientset, namespace, 3, 5*time.Second, writer, false)
			runner.pollInterval = 10 * time.Millisecond

			err := runner.createAndWaitForJob(t.Context(), test.job)

			switch len(test.expectedError) {
			case 0:
				require.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
		})
	}
}

func TestDeleteJob(t *testing.T) {
	t.Parallel()

	namespace := "test-ns"
	fakeClientset := kubefake.NewSimpleClientset(&batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-job",
			Namespace: namespace,
		},
	})

	writer := new(strings.Builder)
	runner := NewPreDeployJobRunner(fakeClientset, namespace, 3, 30*time.Second, writer, false)

	err := runner.deleteJob(t.Context(), "test-job")
	require.NoError(t, err)

	// Verify job is deleted
	_, err = fakeClientset.BatchV1().Jobs(namespace).Get(t.Context(), "test-job", metav1.GetOptions{})
	assert.Error(t, err)
}
