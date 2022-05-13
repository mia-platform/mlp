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
	"sort"
	"strings"
)

type resourceOrder []string

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
		kindOfA := resources[i].GroupVersionKind.Kind
		kindOfB := resources[j].GroupVersionKind.Kind

		aValue, foundA := getOrderFromAnnotationOrKind(orderingMap, resources[i])
		bValue, foundB := getOrderFromAnnotationOrKind(orderingMap, resources[j])

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

func getOrderFromAnnotationOrKind(orderingMap map[string]int, resource Resource) (int, bool) {
	applyBeforeValue, applyBeforeFound := resource.Object.GetAnnotations()[ApplyBeforeAnnotation]
	if applyBeforeFound {
		order := len(orderingMap)

		for _, kind := range strings.Split(applyBeforeValue, ",") {
			kindOrder, kindOrderFound := orderingMap[kind]
			if kindOrderFound && kindOrder < order {
				order = kindOrder - 1
			}
		}

		return order, true
	}

	order, orderFound := orderingMap[resource.GroupVersionKind.Kind]
	return order, orderFound
}
