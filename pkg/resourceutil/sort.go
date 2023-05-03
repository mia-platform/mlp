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

package resourceutil

import (
	"sort"
	"strings"
)

type resourceOrder []string

// Annotation to override resource application order,
// it should be a comma separated list of kinds for which
// this specific resource must be applied before.
// i.e. mia-platform.eu/apply-before-kinds: Job, Deployment, Pod
var applyBeforeAnnotation = GetMiaAnnotation("apply-before-kinds")

var defaultSortOrder = resourceOrder{
	"Namespace",
	"NetworkPolicy",
	"ResourceQuota",
	"LimitRange",
	"PodSecurityPolicy",
	"PodDisruptionBudget",
	"ServiceAccount",
	"ClusterRole",
	"ClusterRoleBinding",
	"Role",
	"RoleBinding",
	"SecretStore",
	"ExternalSecret",
	"SecretProviderClass",
	"Secret",
	"ConfigMap",
	"StorageClass",
	"PersistentVolume",
	"PersistentVolumeClaim",
	"CustomResourceDefinition",
	"Service",
	"DaemonSet",
	"Pod",
	"Deployment",
	"HorizontalPodAutoscaler",
	"StatefulSet",
	"Job",
	"CronJob",
	"Ingress",
	"APIService",
	"Certificate",
	"Middleware",
	"TLSOption",
	"IngressRoute",
	"Flow",
	"Output",
}

// SortResourcesByKind Results are sorted by 'ordering', keeping order of items with equal kind/priority
func SortResourcesByKind(resources []Resource, ordering resourceOrder) []Resource {
	if ordering == nil {
		ordering = defaultSortOrder
	}

	orderingMap := convertOrderingInMap(ordering)
	sort.SliceStable(resources, func(i, j int) bool {
		resA := resources[i]
		resB := resources[j]
		kindOfA := resA.GroupVersionKind.Kind
		kindOfB := resB.GroupVersionKind.Kind

		aValue, foundA := getOrderFromAnnotationOrKind(orderingMap, resA)
		bValue, foundB := getOrderFromAnnotationOrKind(orderingMap, resB)

		// if both kind are unknown to us return an alphabetical sort by kind or do nothing if the kind is the same
		if !foundA && !foundB {
			if kindOfA != kindOfB {
				return kindOfA < kindOfB
			}
			return aValue < bValue
		}

		// if only one of the two kind is unknown keep the unknown at the end
		if !foundA {
			return false
		}
		if !foundB {
			return true
		}

		return aValue < bValue
	})

	return resources
}

// assign to every string inside ordering a value based on its index
func convertOrderingInMap(ordering resourceOrder) map[string]int {
	orderingMap := make(map[string]int, len(ordering))
	for value, key := range ordering {
		orderingMap[key] = value
	}

	return orderingMap
}

// returns the lowest order of the kinds specified in
// mia-platform.eu/apply-before-kinds annotation or, if the annotation is not
// present, returns the order specified in orderingMap for the resource's kind
// P.S. we use decimals for overridden orders to avoid conflicts with defaults.
func getOrderFromAnnotationOrKind(orderingMap map[string]int, resource Resource) (float32, bool) {
	annotations := resource.Object.GetAnnotations()

	if applyBeforeValue, applyBeforeFound := annotations[applyBeforeAnnotation]; applyBeforeFound {
		order := float32(len(orderingMap))

		for _, kind := range strings.Split(applyBeforeValue, ",") {
			trimmedKind := strings.TrimSpace(kind)
			kindOrder, kindOrderFound := orderingMap[trimmedKind]
			kindOrderFloat := float32(kindOrder)

			if kindOrderFound && kindOrderFloat < order {
				order = kindOrderFloat - 0.5
			}
		}

		return order, true
	}

	order, orderFound := orderingMap[resource.GroupVersionKind.Kind]
	return float32(order), orderFound
}
