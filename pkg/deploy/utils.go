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

package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/mia-platform/mlp/pkg/resourceutil"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	resourceSecretName = "resources-deployed"
	resourceField      = "resources"
)

var (
	gvrSecrets         = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}
	gvrNamespaces      = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}
	gvrConfigMaps      = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}
	gvrDeployments     = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	gvrJobs            = schema.GroupVersionResource{Group: batchv1.SchemeGroupVersion.Group, Version: batchv1.SchemeGroupVersion.Version, Resource: "jobs"}
	gvrV1beta1Cronjobs = schema.GroupVersionResource{Group: "batch", Version: "v1beta1", Resource: "cronjobs"}
)

// ResourceList is the base block used to build the secret containing
// the resources deployed in the cluster.
type ResourceList struct {
	Gvk       *schema.GroupVersionKind `json:"kind"`
	Resources []string                 `json:"resources"`
}

// makeResourceMap groups the resources list by kind and embeds them in a `ResourceList` struct
func makeResourceMap(resources []resourceutil.Resource) map[string]*ResourceList {
	res := make(map[string]*ResourceList)

	for _, r := range resources {
		if _, ok := res[r.GroupVersionKind.Kind]; !ok {
			res[r.GroupVersionKind.Kind] = &ResourceList{
				Gvk:       r.GroupVersionKind,
				Resources: []string{},
			}
		}
		res[r.GroupVersionKind.Kind].Resources = append(res[r.GroupVersionKind.Kind].Resources, r.Object.GetName())
	}

	return res
}

// Resources secrets created with helper/builer version of mlp is incompatible with newer versions
// this function convert old format in the new one
func convertSecretFormat(resources []byte) (map[string]*ResourceList, error) {

	type oldResourceList struct {
		Kind      string `json:"kind"`
		Mapping   schema.GroupVersionResource
		Resources []string `json:"resources"`
	}

	oldres := make(map[string]*oldResourceList)
	err := json.Unmarshal(resources, &oldres)
	if err != nil {
		return nil, err
	}

	res := make(map[string]*ResourceList)

	for k, v := range oldres {
		res[k] = &ResourceList{
			Gvk: &schema.GroupVersionKind{
				Group:   v.Mapping.Group,
				Version: v.Mapping.Version,
				Kind:    k,
			},
			Resources: v.Resources}
	}
	return res, nil
}

// getOldResourceMap fetches the last set of resources deployed into the namespace from
// `resourceSecretName` secret.
func getOldResourceMap(clients *k8sClients, namespace string) (map[string]*ResourceList, error) {
	var secret corev1.Secret
	secretUnstr, err := clients.dynamic.Resource(gvrSecrets).
		Namespace(namespace).Get(context.Background(), resourceSecretName, metav1.GetOptions{})

	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]*ResourceList{}, nil
		}
		return nil, err
	}

	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(secretUnstr.Object, &secret)
	if err != nil {
		return nil, err
	}

	res := make(map[string]*ResourceList)

	resources := secret.Data[resourceField]
	if strings.Contains(string(resources), "\"Mapping\":{") {
		res, err = convertSecretFormat(resources)
	} else {
		err = json.Unmarshal(resources, &res)
	}
	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return nil, errors.New("resource field is empty")
	}

	return res, nil
}

// deletedResources returns the resources not contained in the last deploy
func deletedResources(actual, old map[string]*ResourceList) map[string]*ResourceList {
	res := make(map[string]*ResourceList)

	// get diff on already existing resources, the new ones
	// are added with the new secret.
	for key := range old {
		if _, ok := res[key]; !ok {
			res[key] = &ResourceList{
				Gvk: old[key].Gvk,
			}
		}

		if _, ok := actual[key]; ok {
			res[key].Resources = diffResourceArray(actual[key].Resources, old[key].Resources)
		} else {
			res[key].Resources = old[key].Resources
		}
	}

	// Remove entries with empty diff
	for kind, resourceGroup := range res {
		if len(resourceGroup.Resources) == 0 {
			delete(res, kind)
		}
	}

	return res
}

// diffResourceArray returns the old values missing in the new slice
func diffResourceArray(actual, old []string) []string {
	res := []string{}

	for _, oValue := range old {
		if !contains(actual, oValue) {
			res = append(res, oValue)
		}
	}

	return res
}

// contains takes a string slice and search for an element in it.
func contains(res []string, s string) bool {
	for _, item := range res {
		if item == s {
			return true
		}
	}
	return false
}

// convert runtime object to unstructured.Unstructured
func fromRuntimeObjtoUnstruct(obj runtime.Object, gvk schema.GroupVersionKind) (*unstructured.Unstructured, error) {
	currentObj := &unstructured.Unstructured{}
	currentObj.SetGroupVersionKind(gvk)
	interfCurrentObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&obj)
	if err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{
		Object: interfCurrentObj,
	}, nil
}
