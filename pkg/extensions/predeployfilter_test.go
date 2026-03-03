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
	"path/filepath"
	"testing"

	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestPreDeployFilter(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "filter")
	preDeployJob := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "pre-deploy-job.yaml"))
	regularJob := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "regular-job.yaml"))

	tests := map[string]struct {
		object          *unstructured.Unstructured
		annotationValue string
		expected        bool
	}{
		"no filtering for deployment": {
			object:          jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
			annotationValue: PreDeployFilterDefaultValue,
			expected:        false,
		},
		"no filtering for configmap": {
			object:          jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "configmap.yaml")),
			annotationValue: PreDeployFilterDefaultValue,
			expected:        false,
		},
		"no filtering for secret": {
			object:          jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "secret.yaml")),
			annotationValue: PreDeployFilterDefaultValue,
			expected:        false,
		},
		"no filtering for job without pre-deploy annotation": {
			object:          regularJob,
			annotationValue: PreDeployFilterDefaultValue,
			expected:        false,
		},
		"filtering for job with pre-deploy annotation": {
			object:          preDeployJob,
			annotationValue: PreDeployFilterDefaultValue,
			expected:        true,
		},
		"no filtering when annotation value does not match": {
			object:          preDeployJob,
			annotationValue: "custom-value",
			expected:        false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			filter := NewPreDeployFilter(test.annotationValue)
			filtered, err := filter.Filter(test.object, nil)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, filtered)
		})
	}
}

func TestIsPreDeployJob(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "filter")

	tests := map[string]struct {
		object          *unstructured.Unstructured
		annotationValue string
		expected        bool
	}{
		"deployment is not a pre-deploy job": {
			object:          jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml")),
			annotationValue: PreDeployFilterDefaultValue,
			expected:        false,
		},
		"regular job is not a pre-deploy job": {
			object:          jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "regular-job.yaml")),
			annotationValue: PreDeployFilterDefaultValue,
			expected:        false,
		},
		"job with pre-deploy annotation is a pre-deploy job": {
			object:          jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "pre-deploy-job.yaml")),
			annotationValue: PreDeployFilterDefaultValue,
			expected:        true,
		},
		"job with custom annotation value matches": {
			object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]interface{}{
						"name": "custom-job",
						"annotations": map[string]interface{}{
							"mia-platform.eu/deploy": "custom-pre-deploy",
						},
					},
				},
			},
			annotationValue: "custom-pre-deploy",
			expected:        true,
		},
		"job with default annotation does not match custom value": {
			object:          jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "pre-deploy-job.yaml")),
			annotationValue: "custom-pre-deploy",
			expected:        false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.expected, IsPreDeployJob(test.object, test.annotationValue))
		})
	}
}

func TestIsOptionalPreDeployJob(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		object   *unstructured.Unstructured
		expected bool
	}{
		"job without optional annotation is not optional": {
			object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]interface{}{
						"name": "migration",
						"annotations": map[string]interface{}{
							"mia-platform.eu/deploy": "pre-deploy",
						},
					},
				},
			},
			expected: false,
		},
		"job with optional annotation set to true is optional": {
			object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]interface{}{
						"name": "migration",
						"annotations": map[string]interface{}{
							"mia-platform.eu/deploy":          "pre-deploy",
							"mia-platform.eu/deploy-optional": "true",
						},
					},
				},
			},
			expected: true,
		},
		"job with optional annotation set to false is not optional": {
			object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]interface{}{
						"name": "migration",
						"annotations": map[string]interface{}{
							"mia-platform.eu/deploy":          "pre-deploy",
							"mia-platform.eu/deploy-optional": "false",
						},
					},
				},
			},
			expected: false,
		},
		"job without annotations is not optional": {
			object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch/v1",
					"kind":       "Job",
					"metadata": map[string]interface{}{
						"name": "migration",
					},
				},
			},
			expected: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.expected, IsOptionalPreDeployJob(test.object))
		})
	}
}

func TestExtractPreDeployJobs(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "filter")
	preDeployJob := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "pre-deploy-job.yaml"))
	regularJob := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "regular-job.yaml"))
	deployment := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "deployment.yaml"))
	configmap := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "configmap.yaml"))

	customJob := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "batch/v1",
			"kind":       "Job",
			"metadata": map[string]interface{}{
				"name": "custom-job",
				"annotations": map[string]interface{}{
					"mia-platform.eu/deploy": "custom-value",
				},
			},
		},
	}

	tests := map[string]struct {
		resources             []*unstructured.Unstructured
		annotationValue       string
		expectedPreDeployJobs int
		expectedRemaining     int
	}{
		"empty resources": {
			resources:             []*unstructured.Unstructured{},
			annotationValue:       PreDeployFilterDefaultValue,
			expectedPreDeployJobs: 0,
			expectedRemaining:     0,
		},
		"no pre-deploy jobs": {
			resources:             []*unstructured.Unstructured{deployment, configmap, regularJob},
			annotationValue:       PreDeployFilterDefaultValue,
			expectedPreDeployJobs: 0,
			expectedRemaining:     3,
		},
		"only pre-deploy jobs": {
			resources:             []*unstructured.Unstructured{preDeployJob},
			annotationValue:       PreDeployFilterDefaultValue,
			expectedPreDeployJobs: 1,
			expectedRemaining:     0,
		},
		"mixed resources with pre-deploy jobs": {
			resources:             []*unstructured.Unstructured{deployment, preDeployJob, configmap, regularJob},
			annotationValue:       PreDeployFilterDefaultValue,
			expectedPreDeployJobs: 1,
			expectedRemaining:     3,
		},
		"custom annotation value extracts correct jobs": {
			resources:             []*unstructured.Unstructured{deployment, preDeployJob, customJob, regularJob},
			annotationValue:       "custom-value",
			expectedPreDeployJobs: 1,
			expectedRemaining:     3,
		},
		"default annotation value does not match custom jobs": {
			resources:             []*unstructured.Unstructured{customJob, regularJob},
			annotationValue:       PreDeployFilterDefaultValue,
			expectedPreDeployJobs: 0,
			expectedRemaining:     2,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			preDeployJobs, remaining := ExtractPreDeployJobs(test.resources, test.annotationValue)
			assert.Len(t, preDeployJobs, test.expectedPreDeployJobs)
			assert.Len(t, remaining, test.expectedRemaining)
		})
	}
}
