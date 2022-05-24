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

package resourceutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestSortResourcesByKind(t *testing.T) {
	t.Run("Reordering resources based on default reordering function", func(t *testing.T) {
		resources := []Resource{
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Unknown"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Secret"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "ConfigMap"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "ClusterRole"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "IngressRoute"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "ClusterRoleBinding"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "ConfigMap"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Deployment"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "PodSecurityPolicy"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "ServiceAccount"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "SecretProviderClass"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Service"}},
		}
		expected := []string{
			"PodSecurityPolicy",
			"ServiceAccount",
			"ClusterRole",
			"ClusterRoleBinding",
			"SecretProviderClass",
			"Secret",
			"ConfigMap",
			"ConfigMap",
			"Service",
			"Deployment",
			"IngressRoute",
			"Unknown",
		}
		var orderedNames []string
		for _, resource := range SortResourcesByKind(resources, nil) {
			orderedNames = append(orderedNames, resource.GroupVersionKind.Kind)
		}

		require.Exactly(t, expected, orderedNames)
	})

	t.Run("Reordering resources with mia-platform.eu/apply-before-kinds annotation", func(t *testing.T) {
		resources := []Resource{
			makeResourceWithApplyBeforeAnnotation(t, "UnknownBeforeNamespace", "Namespace"),
			makeResourceWithApplyBeforeAnnotation(t, "UnknownBeforeEmpty", ""),
			makeResourceWithApplyBeforeAnnotation(t, "UnknownBeforeDeploymentAndJob", "Job, Deployment"),
			makeResourceWithApplyBeforeAnnotation(t, "UnknownBeforeDeploymentAndNotHandledKind", "Deployment, NotHandledKind"),
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Job"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Pod"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Deployment"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Namespace"}},
			{GroupVersionKind: &schema.GroupVersionKind{Kind: "Unknown"}},
		}

		expected := []string{
			"UnknownBeforeNamespace",
			"Namespace",
			"Pod",
			"UnknownBeforeDeploymentAndJob",
			"UnknownBeforeDeploymentAndNotHandledKind",
			"Deployment",
			"Job",
			"UnknownBeforeEmpty",
			"Unknown",
		}
		var orderedNames []string
		for _, resource := range SortResourcesByKind(resources, nil) {
			orderedNames = append(orderedNames, resource.GroupVersionKind.Kind)
		}

		require.Exactly(t, expected, orderedNames)
	})
}

func makeResourceWithApplyBeforeAnnotation(t *testing.T, kind string, applyBefore string) Resource {
	t.Helper()
	resource := Resource{
		GroupVersionKind: &schema.GroupVersionKind{Kind: kind},
	}
	resource.Object.SetAnnotations(map[string]string{
		"mia-platform.eu/apply-before-kinds": applyBefore,
	})
	return resource
}
