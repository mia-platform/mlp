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

package extensions

import (
	"reflect"

	"github.com/mia-platform/jpl/pkg/client/cache"
	"github.com/mia-platform/jpl/pkg/filter"
	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	// PreDeployFilterDefaultValue is the default value for the deploy annotation that identifies
	// a Job as a pre-deploy job. It can be overridden via the --pre-deploy-job-annotation CLI flag.
	PreDeployFilterDefaultValue = "pre-deploy"

	// preDeployOptionalAnnotation is the annotation used to mark a pre-deploy job as optional.
	// If set to "true", a failure of this job will not block the deploy pipeline.
	preDeployOptionalAnnotation = miaPlatformPrefix + "deploy-optional"
)

var (
	jobGK = schema.GroupKind{Group: batchv1.SchemeGroupVersion.Group, Kind: reflect.TypeOf(batchv1.Job{}).Name()}
)

// preDeployFilter will implement a filter that will remove a Job if it has a matching
// value in the deployFilterAnnotation. These jobs are meant to be executed before
// the main deploy and are handled separately.
type preDeployFilter struct {
	annotationValue string
}

// NewPreDeployFilter return a new filter for intercepting pre-deploy jobs and removing them
// from the main deploy pipeline. The annotationValue parameter specifies the value that the
// deploy annotation must match to identify a pre-deploy job.
func NewPreDeployFilter(annotationValue string) filter.Interface {
	return &preDeployFilter{annotationValue: annotationValue}
}

// Filter implement filter.Interface interface
func (f *preDeployFilter) Filter(obj *unstructured.Unstructured, _ cache.RemoteResourceGetter) (bool, error) {
	return IsPreDeployJob(obj, f.annotationValue), nil
}

// IsPreDeployJob return true if the object is a Job annotated for pre-deploy execution.
// The annotationValue parameter specifies the value that the deploy annotation must match.
func IsPreDeployJob(obj *unstructured.Unstructured, annotationValue string) bool {
	if obj.GroupVersionKind().GroupKind() != jobGK {
		return false
	}

	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}

	return annotations[deployFilterAnnotation] == annotationValue
}

// IsOptionalPreDeployJob return true if the object has the optional annotation set to "true".
// Optional pre-deploy jobs do not block the deploy pipeline if they fail.
func IsOptionalPreDeployJob(obj *unstructured.Unstructured) bool {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		return false
	}

	return annotations[preDeployOptionalAnnotation] == "true"
}

// ExtractPreDeployJobs separates pre-deploy jobs from the resources list, returning
// the pre-deploy jobs and the remaining resources separately. The annotationValue parameter
// specifies the value that the deploy annotation must match to identify a pre-deploy job.
func ExtractPreDeployJobs(resources []*unstructured.Unstructured, annotationValue string) ([]*unstructured.Unstructured, []*unstructured.Unstructured) {
	preDeployJobs := make([]*unstructured.Unstructured, 0)
	remaining := make([]*unstructured.Unstructured, 0, len(resources))

	for _, res := range resources {
		if IsPreDeployJob(res, annotationValue) {
			preDeployJobs = append(preDeployJobs, res)
		} else {
			remaining = append(remaining, res)
		}
	}

	return preDeployJobs, remaining
}

// keep it to always check if preDeployFilter implement correctly the filter.Interface interface
var _ filter.Interface = &preDeployFilter{}
