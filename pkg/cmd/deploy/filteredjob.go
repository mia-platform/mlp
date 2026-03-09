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
	"fmt"
	"io"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
)

const (
	filteredJobAnnotationKey = "mia-platform.eu/deploy"
	deployOptionalAnnotation = "mia-platform.eu/deploy-optional"
	jobKind                  = "Job"
)

// FilteredJobRunner handles the creation, execution and monitoring of filtered jobs
// with configurable retry and timeout support.
type FilteredJobRunner struct {
	clientSet    kubernetes.Interface
	namespace    string
	maxRetries   int
	timeout      time.Duration
	pollInterval time.Duration
	writer       io.Writer
	dryRun       bool
}

// NewFilteredJobRunner creates a new FilteredJobRunner configured with the specified parameters
// for running filtered jobs against the target cluster.
func NewFilteredJobRunner(clientSet kubernetes.Interface, namespace string, maxRetries int, timeout time.Duration, writer io.Writer, dryRun bool) *FilteredJobRunner {
	return &FilteredJobRunner{
		clientSet:    clientSet,
		namespace:    namespace,
		maxRetries:   maxRetries,
		timeout:      timeout,
		pollInterval: 1 * time.Second,
		writer:       writer,
		dryRun:       dryRun,
	}
}

// FilterAnnotatedJobs separates filtered jobs from the remaining resources based on the
// mia-platform.eu/deploy annotation matching the provided annotation value.
// It returns two slices: the matching filtered jobs and the remaining resources.
func FilterAnnotatedJobs(resources []*unstructured.Unstructured, annotationValue string) ([]*unstructured.Unstructured, []*unstructured.Unstructured) {
	var filteredJobs []*unstructured.Unstructured
	var remainingResources []*unstructured.Unstructured

	for _, res := range resources {
		if res.GetKind() == jobKind {
			annotations := res.GetAnnotations()
			if val, ok := annotations[filteredJobAnnotationKey]; ok && val == annotationValue {
				filteredJobs = append(filteredJobs, res)
				continue
			}
		}
		remainingResources = append(remainingResources, res)
	}

	return filteredJobs, remainingResources
}

// StripAnnotatedJobs removes all Job resources that carry the mia-platform.eu/deploy
// annotation, regardless of its value. This is used to exclude filtered jobs from the
// normal apply flow when the --filtered-job-annotation flag is not provided.
func StripAnnotatedJobs(resources []*unstructured.Unstructured) []*unstructured.Unstructured {
	var remaining []*unstructured.Unstructured

	for _, res := range resources {
		if res.GetKind() == jobKind {
			if _, ok := res.GetAnnotations()[filteredJobAnnotationKey]; ok {
				continue
			}
		}
		remaining = append(remaining, res)
	}

	return remaining
}

// isOptionalFilteredJob reports whether the job carries the deploy-optional annotation set to "true".
func isOptionalFilteredJob(job *unstructured.Unstructured) bool {
	return job.GetAnnotations()[deployOptionalAnnotation] == "true"
}

// Run executes all filtered jobs with retry and timeout support. Each job is retried
// up to maxRetries times upon failure. Jobs annotated with mia-platform.eu/deploy-optional=true
// are treated as non-blocking: their failure is logged as a warning but never counted as an
// error. For mandatory jobs an error is returned only if ALL of them fail; if at least one
// mandatory job succeeds, the deploy process can continue.
func (r *FilteredJobRunner) Run(ctx context.Context, jobs []*unstructured.Unstructured) error {
	logger := logr.FromContextOrDiscard(ctx)

	if len(jobs) == 0 {
		logger.V(3).Info("no filtered jobs to run")
		return nil
	}

	if r.dryRun {
		for _, job := range jobs {
			fmt.Fprintf(r.writer, "filtered job %q would be executed (dry-run)\n", job.GetName())
		}
		return nil
	}

	logger.V(3).Info("starting filtered jobs", "count", len(jobs))

	for _, job := range jobs {
		jobName := job.GetName()
		optional := isOptionalFilteredJob(job)

		err := r.runJobWithRetries(ctx, job)
		if err != nil {
			if optional {
				logger.V(3).Info("optional filtered job failed, continuing", "name", jobName, "error", err)
				fmt.Fprintf(r.writer, "optional filtered job %q failed, continuing\n", jobName)
			} else {
				logger.V(3).Info("filtered job failed", "name", jobName, "error", err)
				return fmt.Errorf("filtered job %q failed after %d attempts: %w", jobName, r.maxRetries, err)
			}
		} else {
			fmt.Fprintf(r.writer, "filtered job %q completed successfully\n", jobName)
		}
	}

	logger.V(3).Info("filtered jobs completed")
	return nil
}

// runJobWithRetries attempts to run a single filtered job, retrying up to maxRetries times
// on failure. The failed job is deleted before each retry attempt.
func (r *FilteredJobRunner) runJobWithRetries(ctx context.Context, jobUnstr *unstructured.Unstructured) error {
	logger := logr.FromContextOrDiscard(ctx)
	var lastErr error

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			logger.V(3).Info("retrying filtered job", "name", jobUnstr.GetName(), "attempt", attempt)
			fmt.Fprintf(r.writer, "retrying filtered job %q (attempt %d/%d)\n", jobUnstr.GetName(), attempt, r.maxRetries)
		}

		lastErr = r.createAndWaitForJob(ctx, jobUnstr)
		if lastErr == nil {
			return nil
		}

		logger.V(5).Info("filtered job attempt failed", "name", jobUnstr.GetName(), "attempt", attempt, "error", lastErr)

		// Clean up the failed job before retrying
		if cleanErr := r.deleteJob(ctx, jobUnstr.GetName()); cleanErr != nil {
			logger.V(5).Info("failed to clean up job", "name", jobUnstr.GetName(), "error", cleanErr)
		}
	}

	return lastErr
}

// createAndWaitForJob creates a single job in the cluster and waits for its completion.
// It converts the unstructured resource to a typed Job, sets the namespace, and monitors
// the job status until completion, failure, or timeout.
func (r *FilteredJobRunner) createAndWaitForJob(ctx context.Context, jobUnstr *unstructured.Unstructured) error {
	logger := logr.FromContextOrDiscard(ctx)

	var job batchv1.Job
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(jobUnstr.Object, &job); err != nil {
		return fmt.Errorf("failed to convert job %q: %w", jobUnstr.GetName(), err)
	}

	job.Namespace = r.namespace
	// Clear resource version and status for creation
	job.ResourceVersion = ""
	job.Status = batchv1.JobStatus{}

	logger.V(5).Info("creating filtered job", "name", job.Name, "namespace", r.namespace)

	if _, err := r.clientSet.BatchV1().Jobs(r.namespace).Create(ctx, &job, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create job %q: %w", job.Name, err)
	}

	return r.waitForJobCompletion(ctx, job.Name)
}

// waitForJobCompletion polls the job status at regular intervals until the job completes,
// fails, or the configured timeout expires.
func (r *FilteredJobRunner) waitForJobCompletion(ctx context.Context, name string) error {
	logger := logr.FromContextOrDiscard(ctx)

	timeoutCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	ticker := time.NewTicker(r.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("job %q timed out after %s", name, r.timeout)
		case <-ticker.C:
			if timeoutCtx.Err() != nil {
				return fmt.Errorf("job %q timed out after %s", name, r.timeout)
			}
			job, err := r.clientSet.BatchV1().Jobs(r.namespace).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return fmt.Errorf("failed to get job %q status: %w", name, err)
			}

			logger.V(10).Info("polling job status", "name", name, "active", job.Status.Active, "succeeded", job.Status.Succeeded, "failed", job.Status.Failed)

			for _, condition := range job.Status.Conditions {
				if condition.Type == batchv1.JobComplete && condition.Status == corev1.ConditionTrue {
					return nil
				}
				if condition.Type == batchv1.JobFailed && condition.Status == corev1.ConditionTrue {
					return fmt.Errorf("job %q failed: %s", name, condition.Message)
				}
			}
		}
	}
}

// deleteJob removes a job and its associated pods from the cluster using background propagation
func (r *FilteredJobRunner) deleteJob(ctx context.Context, name string) error {
	propagation := metav1.DeletePropagationBackground
	return r.clientSet.BatchV1().Jobs(r.namespace).Delete(ctx, name, metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	})
}
