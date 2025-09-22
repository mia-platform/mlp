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

	"github.com/mia-platform/jpl/pkg/poller"
	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExtendedPoller(t *testing.T) {
	t.Parallel()

	customCheckers := ExternalSecretStatusCheckers()
	assert.Len(t, customCheckers, 2)
}

func TestExternalSecretStatusChecker(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "custom-pollers")
	tests := map[string]struct {
		object         *unstructured.Unstructured
		expectedResult *poller.Result
		expectedError  string
	}{
		"resource secret deleted condition is terminating": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "extsec-terminating.yaml")),
			expectedResult: &poller.Result{
				Status:  poller.StatusTerminating,
				Message: "custom message",
			},
		},
		"resource with ready condition true is current": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "extsec-ready-true.yaml")),
			expectedResult: &poller.Result{
				Status:  poller.StatusCurrent,
				Message: "custom message",
			},
		},
		"resource with ready condition false is in progress": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "extsec-ready-false.yaml")),
			expectedResult: &poller.Result{
				Status:  poller.StatusInProgress,
				Message: "custom message",
			},
		},
		"resource without status is in progress": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "extsec-no-status.yaml")),
			expectedResult: &poller.Result{
				Status:  poller.StatusInProgress,
				Message: "ExternalSecret sync is in progress",
			},
		},
	}

	for testName, testCase := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			result, err := externalSecretStatusChecker(testCase.object)
			if len(testCase.expectedError) > 0 {
				assert.ErrorContains(t, err, testCase.expectedError)
				assert.Equal(t, testCase.expectedResult, result)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedResult, result)
		})
	}
}

func TestSecretStoreStatusChecker(t *testing.T) {
	t.Parallel()

	testdata := filepath.Join("testdata", "custom-pollers")
	tests := map[string]struct {
		object         *unstructured.Unstructured
		expectedResult *poller.Result
		expectedError  string
	}{
		"resource with ready condition true is current": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "secstore-ready-true.yaml")),
			expectedResult: &poller.Result{
				Status:  poller.StatusCurrent,
				Message: "custom message",
			},
		},
		"resource with ready condition false is in progress": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "secstore-ready-false.yaml")),
			expectedResult: &poller.Result{
				Status:  poller.StatusInProgress,
				Message: "custom message",
			},
		},
		"resource without status is in progress": {
			object: jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "secstore-no-status.yaml")),
			expectedResult: &poller.Result{
				Status:  poller.StatusInProgress,
				Message: "SecretStore is in progress",
			},
		},
	}

	for testName, testCase := range tests {
		t.Run(testName, func(t *testing.T) {
			t.Parallel()

			result, err := secretStoreStatusChecker(testCase.object)
			if len(testCase.expectedError) > 0 {
				assert.ErrorContains(t, err, testCase.expectedError)
				assert.Equal(t, testCase.expectedResult, result)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedResult, result)
		})
	}
}
