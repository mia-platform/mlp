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
	extsecv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
	"github.com/mia-platform/jpl/pkg/poller"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func ExternalSecretStatusCheckers() poller.CustomStatusCheckers {
	return poller.CustomStatusCheckers{
		extsecGK:      externalSecretStatusChecker,
		extSecStoreGK: secretStoreStatusChecker,
	}
}

// externalSecretStatusChecker contains the logic for checking if an ExternalSecret has finished to sync its data
func externalSecretStatusChecker(object *unstructured.Unstructured) (*poller.Result, error) {
	externalSecret := new(extsecv1.ExternalSecret)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, externalSecret); err != nil {
		return nil, err
	}

	for _, condition := range externalSecret.Status.Conditions {
		switch condition.Type {
		case extsecv1.ExternalSecretReady:
			switch condition.Status {
			case corev1.ConditionTrue:
				return &poller.Result{
					Status:  poller.StatusCurrent,
					Message: condition.Message,
				}, nil
			case corev1.ConditionFalse:
				return &poller.Result{
					Status:  poller.StatusInProgress,
					Message: condition.Message,
				}, nil
			case corev1.ConditionUnknown:
				return &poller.Result{
					Status:  poller.StatusInProgress,
					Message: condition.Message,
				}, nil
			}
		case extsecv1.ExternalSecretDeleted:
			if condition.Status == corev1.ConditionTrue {
				return &poller.Result{
					Status:  poller.StatusTerminating,
					Message: condition.Message,
				}, nil
			}
		}
	}

	return &poller.Result{
		Status:  poller.StatusInProgress,
		Message: "ExternalSecret sync is in progress",
	}, nil
}

// secretStoreStatusChecker contains the logic for checking if an SecretStore has
func secretStoreStatusChecker(object *unstructured.Unstructured) (*poller.Result, error) {
	secretStore := new(extsecv1.SecretStore)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(object.Object, secretStore); err != nil {
		return nil, err
	}

	for _, condition := range secretStore.Status.Conditions {
		if condition.Type != extsecv1.SecretStoreReady {
			continue
		}
		switch condition.Status {
		case corev1.ConditionTrue:
			return &poller.Result{
				Status:  poller.StatusCurrent,
				Message: condition.Message,
			}, nil
		case corev1.ConditionFalse:
			return &poller.Result{
				Status:  poller.StatusInProgress,
				Message: condition.Message,
			}, nil
		case corev1.ConditionUnknown:
			return &poller.Result{
				Status:  poller.StatusInProgress,
				Message: condition.Message,
			}, nil
		}
	}

	return &poller.Result{
		Status:  poller.StatusInProgress,
		Message: "SecretStore is in progress",
	}, nil
}
