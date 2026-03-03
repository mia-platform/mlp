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
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"

	"github.com/mia-platform/mlp/v2/pkg/extensions"
)

const (
	preDeployPollingInterval = 5 * time.Second
)

// runPreDeployJobs applies each pre-deploy job to the cluster and waits for its completion.
// Each job execution has its own timeout (preDeployJobTimeout). If a job fails, it is retried
// up to preDeployJobMaxRetries times. If all retries are exhausted and the job is not marked
// as optional (via the mia-platform.eu/deploy-optional annotation), the deploy process is aborted.
// Optional jobs that fail are logged as warnings and do not block the deploy pipeline.
// In dry-run mode, jobs are applied with the dry-run option and polling is skipped.
func (o *Options) runPreDeployJobs(ctx context.Context, jobs []*unstructured.Unstructured, namespace string) error {
	if len(jobs) == 0 {
		return nil
	}

	logger := logr.FromContextOrDiscard(ctx)

	pollingInterval := preDeployPollingInterval
	if o.preDeployPollingInterval > 0 {
		pollingInterval = o.preDeployPollingInterval
	}

	maxRetries := o.preDeployJobMaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	dynamicClient, err := o.clientFactory.DynamicClient()
	if err != nil {
		return fmt.Errorf("failed to create dynamic client for pre-deploy jobs: %w", err)
	}

	mapper, err := o.clientFactory.ToRESTMapper()
	if err != nil {
		return fmt.Errorf("failed to create REST mapper for pre-deploy jobs: %w", err)
	}

	jobGVK := batchv1.SchemeGroupVersion.WithKind("Job")
	mapping, err := mapper.RESTMapping(jobGVK.GroupKind(), jobGVK.Version)
	if err != nil {
		return fmt.Errorf("failed to get REST mapping for Job: %w", err)
	}

	jobResource := dynamicClient.Resource(mapping.Resource)

	for _, job := range jobs {
		jobName := job.GetName()
		jobNamespace := job.GetNamespace()
		if jobNamespace == "" {
			jobNamespace = namespace
		}

		lastErr := o.executePreDeployJobWithRetries(ctx, jobResource, job, jobName, jobNamespace, maxRetries, pollingInterval, logger)
		if lastErr != nil {
			if extensions.IsOptionalPreDeployJob(job) {
				fmt.Fprintf(o.writer, "pre-deploy job %s: optional job failed after %d attempt(s), continuing: %s\n", jobName, maxRetries, lastErr)
				continue
			}
			return fmt.Errorf("pre-deploy job %s: failed after %d attempt(s): %w", jobName, maxRetries, lastErr)
		}
	}

	return nil
}

// executePreDeployJobWithRetries runs a single pre-deploy job with the retry logic.
// Returns the last error if all retries failed, or nil on success.
func (o *Options) executePreDeployJobWithRetries(ctx context.Context, jobResource dynamic.NamespaceableResourceInterface, job *unstructured.Unstructured, jobName, jobNamespace string, maxRetries int, pollingInterval time.Duration, logger logr.Logger) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		logger.V(3).Info("running pre-deploy job", "name", jobName, "namespace", jobNamespace, "attempt", attempt)
		fmt.Fprintf(o.writer, "pre-deploy job %s: starting (attempt %d/%d)\n", jobName, attempt, maxRetries)

		if err := o.applyPreDeployJob(ctx, jobResource, job, jobNamespace); err != nil {
			return fmt.Errorf("pre-deploy job %s: failed to apply: %w", jobName, err)
		}

		if o.dryRun {
			fmt.Fprintf(o.writer, "pre-deploy job %s: applied (dry-run)\n", jobName)
			return nil
		}

		// Create a per-execution timeout context
		execCtx := ctx
		if o.preDeployJobTimeout > 0 {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, o.preDeployJobTimeout)
			defer cancel()
		}

		lastErr = o.waitForJobCompletion(execCtx, jobResource, jobName, jobNamespace, pollingInterval)
		if lastErr == nil {
			fmt.Fprintf(o.writer, "pre-deploy job %s: completed successfully\n", jobName)
			return nil
		}

		fmt.Fprintf(o.writer, "pre-deploy job %s: attempt %d/%d failed: %s\n", jobName, attempt, maxRetries, lastErr)
	}
	return lastErr
}

// applyPreDeployJob removes any existing job with the same name and applies the new one using server-side apply.
// It enforces backoffLimit=0 so the job fails immediately on the first pod failure without Kubernetes-level retries.
// Higher-level retries are handled by runPreDeployJobs.
func (o *Options) applyPreDeployJob(ctx context.Context, jobResource dynamic.NamespaceableResourceInterface, job *unstructured.Unstructured, namespace string) error {
	logger := logr.FromContextOrDiscard(ctx)

	// Deep copy to avoid mutating the original object
	job = job.DeepCopy()
	// Force backoffLimit=0: on failure the job fails immediately and we handle retries ourselves
	if err := unstructured.SetNestedField(job.Object, int64(0), "spec", "backoffLimit"); err != nil {
		return fmt.Errorf("failed to set backoffLimit: %w", err)
	}

	if !o.dryRun {
		// Delete any existing job with the same name to ensure a clean run
		propagationPolicy := metav1.DeletePropagationBackground
		err := jobResource.Namespace(namespace).Delete(ctx, job.GetName(), metav1.DeleteOptions{
			PropagationPolicy: &propagationPolicy,
		})
		if err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete existing job: %w", err)
		}

		if err == nil {
			logger.V(5).Info("deleted existing pre-deploy job", "name", job.GetName(), "namespace", namespace)
		}
	}

	data, err := json.Marshal(job.Object)
	if err != nil {
		return fmt.Errorf("failed to marshal job: %w", err)
	}

	patchOpts := metav1.PatchOptions{
		FieldManager: fieldManager,
	}
	if o.dryRun {
		patchOpts.DryRun = []string{metav1.DryRunAll}
	}

	_, err = jobResource.Namespace(namespace).Patch(ctx, job.GetName(), types.ApplyPatchType, data, patchOpts)
	if err != nil {
		return err
	}

	return nil
}

// waitForJobCompletion polls the job status until it completes, fails, or the context times out.
// The status is checked immediately after calling this function, then at each polling interval.
func (o *Options) waitForJobCompletion(ctx context.Context, jobResource dynamic.NamespaceableResourceInterface, name, namespace string, pollingInterval time.Duration) error {
	logger := logr.FromContextOrDiscard(ctx)

	ticker := time.NewTicker(pollingInterval)
	defer ticker.Stop()

	for {
		// Check status immediately on each iteration (including the first one)
		logger.V(5).Info("polling pre-deploy job status", "name", name, "namespace", namespace)

		obj, err := jobResource.Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get job status: %w", err)
		}

		completed, errMsg, err := checkJobStatus(obj)
		if err != nil {
			return err
		}

		if errMsg != "" {
			return fmt.Errorf("job failed: %s", errMsg)
		}

		if completed {
			return nil
		}

		fmt.Fprintf(o.writer, "pre-deploy job %s: waiting for completion\n", name)

		// Wait for next poll interval or context cancellation
		select {
		case <-ctx.Done():
			return errors.New("timed out waiting for job completion")
		case <-ticker.C:
			// continue to next check
		}
	}
}

// checkJobStatus examines the job's status conditions to determine if it has completed or failed.
// Returns (completed, failMessage, error)
func checkJobStatus(obj *unstructured.Unstructured) (bool, string, error) {
	job := new(batchv1.Job)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, job); err != nil {
		return false, "", fmt.Errorf("failed to convert job status: %w", err)
	}

	for _, condition := range job.Status.Conditions {
		switch condition.Type {
		case batchv1.JobComplete:
			if condition.Status == "True" {
				return true, "", nil
			}
		case batchv1.JobFailed:
			if condition.Status == "True" {
				msg := condition.Message
				if msg == "" {
					msg = condition.Reason
				}
				if msg == "" {
					msg = "job execution failed"
				}
				return false, msg, nil
			}
		}
	}

	// Detect failure before the controller sets the Failed condition:
	// if there are no active pods and the number of failures exceeds the backoff limit,
	// the job has effectively failed
	if job.Status.Active == 0 && job.Status.Failed > 0 {
		backoffLimit := int32(6) // Kubernetes default
		if job.Spec.BackoffLimit != nil {
			backoffLimit = *job.Spec.BackoffLimit
		}
		if job.Status.Failed > backoffLimit {
			return false, fmt.Sprintf("job has %d failed pod(s) exceeding backoff limit of %d", job.Status.Failed, backoffLimit), nil
		}
	}

	return false, "", nil
}
