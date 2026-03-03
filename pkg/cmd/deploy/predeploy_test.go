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

	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

func TestCheckJobStatus(t *testing.T) {
	t.Parallel()

	backoffLimitZero := int32(0)

	tests := map[string]struct {
		job             *batchv1.Job
		expectedDone    bool
		expectedFailMsg string
		expectedError   string
	}{
		"job without conditions is in progress": {
			job: &batchv1.Job{
				Status: batchv1.JobStatus{},
			},
			expectedDone:    false,
			expectedFailMsg: "",
		},
		"job with complete condition is done": {
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: "True",
						},
					},
				},
			},
			expectedDone:    true,
			expectedFailMsg: "",
		},
		"job with failed condition returns failure message": {
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:    batchv1.JobFailed,
							Status:  "True",
							Message: "BackoffLimitExceeded",
						},
					},
				},
			},
			expectedDone:    false,
			expectedFailMsg: "BackoffLimitExceeded",
		},
		"job with failed condition and empty message uses reason": {
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: "True",
							Reason: "DeadlineExceeded",
						},
					},
				},
			},
			expectedDone:    false,
			expectedFailMsg: "DeadlineExceeded",
		},
		"job with failed condition and empty message and reason uses default": {
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobFailed,
							Status: "True",
						},
					},
				},
			},
			expectedDone:    false,
			expectedFailMsg: "job execution failed",
		},
		"job with incomplete condition is in progress": {
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Conditions: []batchv1.JobCondition{
						{
							Type:   batchv1.JobComplete,
							Status: "False",
						},
					},
				},
			},
			expectedDone:    false,
			expectedFailMsg: "",
		},
		"job with failed pods exceeding backoff limit detected early": {
			job: &batchv1.Job{
				Spec: batchv1.JobSpec{
					BackoffLimit: &backoffLimitZero,
				},
				Status: batchv1.JobStatus{
					Failed: 1,
					Active: 0,
				},
			},
			expectedDone:    false,
			expectedFailMsg: "job has 1 failed pod(s) exceeding backoff limit of 0",
		},
		"job with failed pods but still active is in progress": {
			job: &batchv1.Job{
				Status: batchv1.JobStatus{
					Failed: 1,
					Active: 1,
				},
			},
			expectedDone:    false,
			expectedFailMsg: "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			unstrObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(test.job)
			require.NoError(t, err)
			obj := &unstructured.Unstructured{Object: unstrObj}

			done, failMsg, err := checkJobStatus(obj)
			if len(test.expectedError) > 0 {
				assert.ErrorContains(t, err, test.expectedError)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, test.expectedDone, done)
			assert.Equal(t, test.expectedFailMsg, failMsg)
		})
	}
}

func TestRunPreDeployJobs(t *testing.T) {
	t.Parallel()

	namespace := "mlp-predeploy-test"

	tests := map[string]struct {
		jobs           []*unstructured.Unstructured
		dryRun         bool
		maxRetries     int
		setupClient    func() *dynamicfake.FakeDynamicClient
		expectedError  string
		expectedOutput []string
	}{
		"no jobs does nothing": {
			jobs:       []*unstructured.Unstructured{},
			maxRetries: 3,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				return dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
			},
		},
		"dry-run applies job without polling": {
			jobs:       []*unstructured.Unstructured{makePreDeployJob("migration")},
			dryRun:     true,
			maxRetries: 3,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, makePreDeployJob("migration"), nil
				})
				return client
			},
			expectedOutput: []string{
				"pre-deploy job migration: starting (attempt 1/3)",
				"pre-deploy job migration: applied (dry-run)",
			},
		},
		"successful job completion on first attempt": {
			jobs:       []*unstructured.Unstructured{makePreDeployJob("migration")},
			maxRetries: 3,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "migration")
				})
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, makePreDeployJob("migration"), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, completedJobUnstructured("migration", namespace), nil
				})
				return client
			},
			expectedOutput: []string{
				"pre-deploy job migration: starting (attempt 1/3)",
				"pre-deploy job migration: completed successfully",
			},
		},
		"failed job exhausts all retries": {
			jobs:       []*unstructured.Unstructured{makePreDeployJob("migration")},
			maxRetries: 3,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "migration")
				})
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, makePreDeployJob("migration"), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, failedJobUnstructured("migration", namespace), nil
				})
				return client
			},
			expectedError: "pre-deploy job migration: failed after 3 attempt(s): job failed: BackoffLimitExceeded",
			expectedOutput: []string{
				"pre-deploy job migration: starting (attempt 1/3)",
				"pre-deploy job migration: attempt 1/3 failed",
				"pre-deploy job migration: starting (attempt 2/3)",
				"pre-deploy job migration: attempt 2/3 failed",
				"pre-deploy job migration: starting (attempt 3/3)",
				"pre-deploy job migration: attempt 3/3 failed",
			},
		},
		"failed job succeeds on retry": {
			jobs:       []*unstructured.Unstructured{makePreDeployJob("migration")},
			maxRetries: 3,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "migration")
				})
				attemptCount := 0
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					attemptCount++
					return true, makePreDeployJob("migration"), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					// First attempt fails, second succeeds
					if attemptCount <= 1 {
						return true, failedJobUnstructured("migration", namespace), nil
					}
					return true, completedJobUnstructured("migration", namespace), nil
				})
				return client
			},
			expectedOutput: []string{
				"pre-deploy job migration: starting (attempt 1/3)",
				"pre-deploy job migration: attempt 1/3 failed",
				"pre-deploy job migration: starting (attempt 2/3)",
				"pre-deploy job migration: completed successfully",
			},
		},
		"single retry allowed fails immediately": {
			jobs:       []*unstructured.Unstructured{makePreDeployJob("migration")},
			maxRetries: 1,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "migration")
				})
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, makePreDeployJob("migration"), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, failedJobUnstructured("migration", namespace), nil
				})
				return client
			},
			expectedError: "pre-deploy job migration: failed after 1 attempt(s): job failed: BackoffLimitExceeded",
			expectedOutput: []string{
				"pre-deploy job migration: starting (attempt 1/1)",
				"pre-deploy job migration: attempt 1/1 failed",
			},
		},
		"timeout on single execution retries": {
			jobs:       []*unstructured.Unstructured{makePreDeployJob("migration")},
			maxRetries: 2,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "migration")
				})
				attemptCount := 0
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					attemptCount++
					return true, makePreDeployJob("migration"), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					// First attempt always returns in-progress to force timeout,
					// second attempt succeeds immediately
					if attemptCount <= 1 {
						return true, inProgressJobUnstructured("migration", namespace), nil
					}
					return true, completedJobUnstructured("migration", namespace), nil
				})
				return client
			},
			expectedOutput: []string{
				"pre-deploy job migration: starting (attempt 1/2)",
				"pre-deploy job migration: attempt 1/2 failed: timed out waiting for job completion",
				"pre-deploy job migration: starting (attempt 2/2)",
				"pre-deploy job migration: completed successfully",
			},
		},
		"apply failure returns error without retry": {
			jobs:       []*unstructured.Unstructured{makePreDeployJob("migration")},
			maxRetries: 3,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "migration")
				})
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, errors.New("forbidden")
				})
				return client
			},
			expectedError: "pre-deploy job migration: failed to apply",
		},
		"delete existing job before apply": {
			jobs:       []*unstructured.Unstructured{makePreDeployJob("migration")},
			maxRetries: 3,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, nil
				})
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, makePreDeployJob("migration"), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, completedJobUnstructured("migration", namespace), nil
				})
				return client
			},
			expectedOutput: []string{
				"pre-deploy job migration: starting (attempt 1/3)",
				"pre-deploy job migration: completed successfully",
			},
		},
		"optional job failure does not block deploy": {
			jobs:       []*unstructured.Unstructured{makeOptionalPreDeployJob("optional-migration")},
			maxRetries: 2,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "optional-migration")
				})
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, makeOptionalPreDeployJob("optional-migration"), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, failedJobUnstructured("optional-migration", namespace), nil
				})
				return client
			},
			expectedOutput: []string{
				"pre-deploy job optional-migration: starting (attempt 1/2)",
				"pre-deploy job optional-migration: attempt 1/2 failed",
				"pre-deploy job optional-migration: starting (attempt 2/2)",
				"pre-deploy job optional-migration: attempt 2/2 failed",
				"pre-deploy job optional-migration: optional job failed after 2 attempt(s), continuing",
			},
		},
		"optional job followed by required job continues on optional failure": {
			jobs: []*unstructured.Unstructured{
				makeOptionalPreDeployJob("optional-migration"),
				makePreDeployJob("required-migration"),
			},
			maxRetries: 1,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "job")
				})
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					patchAction := action.(k8stesting.PatchAction)
					name := patchAction.GetName()
					if name == "optional-migration" {
						return true, makeOptionalPreDeployJob(name), nil
					}
					return true, makePreDeployJob(name), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					getAction := action.(k8stesting.GetAction)
					name := getAction.GetName()
					if name == "optional-migration" {
						return true, failedJobUnstructured(name, namespace), nil
					}
					return true, completedJobUnstructured(name, namespace), nil
				})
				return client
			},
			expectedOutput: []string{
				"pre-deploy job optional-migration: starting (attempt 1/1)",
				"pre-deploy job optional-migration: optional job failed after 1 attempt(s), continuing",
				"pre-deploy job required-migration: starting (attempt 1/1)",
				"pre-deploy job required-migration: completed successfully",
			},
		},
		"optional job success proceeds normally": {
			jobs:       []*unstructured.Unstructured{makeOptionalPreDeployJob("optional-migration")},
			maxRetries: 3,
			setupClient: func() *dynamicfake.FakeDynamicClient {
				client := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme)
				client.PrependReactor("delete", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, nil, k8serrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, "optional-migration")
				})
				client.PrependReactor("patch", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, makeOptionalPreDeployJob("optional-migration"), nil
				})
				client.PrependReactor("get", "jobs", func(action k8stesting.Action) (bool, runtime.Object, error) {
					return true, completedJobUnstructured("optional-migration", namespace), nil
				})
				return client
			},
			expectedOutput: []string{
				"pre-deploy job optional-migration: starting (attempt 1/3)",
				"pre-deploy job optional-migration: completed successfully",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			stringBuilder := new(strings.Builder)

			tf := jpltesting.NewTestClientFactory().
				WithNamespace(namespace)

			if test.setupClient != nil {
				tf.FakeDynamicClient = test.setupClient()
			}

			// Add batch/v1 Job mapping to the REST mapper
			batchGV := batchv1.SchemeGroupVersion
			batchMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{batchGV})
			batchMapper.AddSpecific(
				batchGV.WithKind("Job"),
				batchGV.WithResource("jobs"),
				batchGV.WithResource("job"),
				meta.RESTScopeNamespace,
			)
			tf.RESTMapper = batchMapper

			// Use a short timeout for tests
			ctx := t.Context()

			options := &Options{
				dryRun:                   test.dryRun,
				writer:                   stringBuilder,
				clientFactory:            tf,
				preDeployPollingInterval: 100 * time.Millisecond,
				preDeployJobTimeout:      500 * time.Millisecond,
				preDeployJobMaxRetries:   test.maxRetries,
			}

			err := options.runPreDeployJobs(ctx, test.jobs, namespace)
			output := stringBuilder.String()
			t.Log(output)

			switch len(test.expectedError) {
			case 0:
				require.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}

			for _, expected := range test.expectedOutput {
				assert.Contains(t, output, expected)
			}
		})
	}
}

func makePreDeployJob(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name": name,
				"annotations": map[string]interface{}{
					"mia-platform.eu/deploy": "pre-deploy",
				},
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "migration",
								"image": "busybox",
								"args":  []interface{}{"/bin/sh", "-c", "echo running"},
							},
						},
						"restartPolicy": "Never",
					},
				},
			},
		},
	}
}

func makeOptionalPreDeployJob(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name": name,
				"annotations": map[string]interface{}{
					"mia-platform.eu/deploy":          "pre-deploy",
					"mia-platform.eu/deploy-optional": "true",
				},
			},
			"spec": map[string]interface{}{
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"containers": []interface{}{
							map[string]interface{}{
								"name":  "migration",
								"image": "busybox",
								"args":  []interface{}{"/bin/sh", "-c", "echo running"},
							},
						},
						"restartPolicy": "Never",
					},
				},
			},
		},
	}
}

func completedJobUnstructured(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   string(batchv1.JobComplete),
						"status": "True",
					},
				},
			},
		},
	}
}

func failedJobUnstructured(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    string(batchv1.JobFailed),
						"status":  "True",
						"message": "BackoffLimitExceeded",
					},
				},
			},
		},
	}
}

func inProgressJobUnstructured(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": namespace,
			},
			"status": map[string]interface{}{},
		},
	}
}
