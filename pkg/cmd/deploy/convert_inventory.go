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
	"fmt"

	"github.com/mia-platform/jpl/pkg/inventory"
	"github.com/mia-platform/jpl/pkg/resource"
	"github.com/mia-platform/jpl/pkg/util"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
)

const (
	oldInventoryName = "resources-deployed"
	oldInventoryKey  = "resources"
)

// Inventory wrap
type Inventory struct {
	delegate  inventory.Store
	namespace string

	compatibilityMode bool

	clientset kubernetes.Interface
	mapper    meta.RESTMapper
}

func NewInventory(factory util.ClientFactory, name, namespace, filedManager string) (inventory.Store, error) {
	cmInventory, err := inventory.NewConfigMapStore(factory, name, namespace, filedManager)
	if err != nil {
		return nil, err
	}

	clientset, err := factory.KubernetesClientSet()
	if err != nil {
		return nil, err
	}

	mapper, err := factory.ToRESTMapper()
	if err != nil {
		return nil, err
	}

	return &Inventory{
		delegate:  cmInventory,
		namespace: namespace,

		compatibilityMode: true,

		clientset: clientset,
		mapper:    mapper,
	}, nil
}

func (s *Inventory) Load(ctx context.Context) (sets.Set[resource.ObjectMetadata], error) {
	objs, err := s.delegate.Load(ctx)
	if err != nil || len(objs) > 0 {
		s.compatibilityMode = false
	}

	if s.compatibilityMode {
		return s.oldInventoryObjects(ctx)
	}

	return objs, err
}

func (s *Inventory) Save(ctx context.Context, dryRun bool) error {
	if err := s.delegate.Save(ctx, dryRun); err != nil || !s.compatibilityMode {
		return err
	}

	return s.deleteOldInventory(ctx, dryRun)
}

func (s *Inventory) Delete(ctx context.Context, dryRun bool) error {
	return s.delegate.Delete(ctx, dryRun)
}

func (s *Inventory) SetObjects(objects sets.Set[*unstructured.Unstructured]) {
	s.delegate.SetObjects(objects)
}

func (s *Inventory) oldInventoryObjects(ctx context.Context) (sets.Set[resource.ObjectMetadata], error) {
	metadataSet := make(sets.Set[resource.ObjectMetadata], 0)
	sec, err := s.clientset.CoreV1().Secrets(s.namespace).Get(ctx, oldInventoryName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			s.compatibilityMode = false
			return metadataSet, nil
		}
		return nil, fmt.Errorf("failed to find inventory: %w", err)
	}

	secretData := sec.Data[oldInventoryKey]
	resourceList := make(map[string]*resourceList)

	if err := json.Unmarshal(secretData, &resourceList); err != nil {
		var convertError error
		resourceList, convertError = convertSecretFormat(secretData)
		if convertError != nil {
			return nil, fmt.Errorf("error unmarshalling resource map in secret %s: %s in namespace %s. Try to convert format from v0, but fails", oldInventoryName, err, s.namespace)
		}
	}

	set := make(sets.Set[resource.ObjectMetadata])
	for _, list := range resourceList {
		gvk := list.Gvk

		mapping, err := s.mapper.RESTMapping(gvk.GroupKind())
		if err != nil {
			return nil, err
		}

		namespace := ""
		if mapping.Scope == meta.RESTScopeNamespace {
			namespace = s.namespace
		}
		for _, name := range list.Resources {
			set.Insert(resource.ObjectMetadata{
				Name:      name,
				Namespace: namespace,
				Group:     gvk.Group,
				Kind:      gvk.Kind,
			})
		}
	}

	return set, nil
}

func (s *Inventory) deleteOldInventory(ctx context.Context, dryRun bool) error {
	if dryRun {
		return nil
	}

	s.compatibilityMode = false
	propagation := metav1.DeletePropagationBackground
	opts := metav1.DeleteOptions{
		PropagationPolicy: &propagation,
	}

	if err := s.clientset.CoreV1().Secrets(s.namespace).Delete(ctx, oldInventoryName, opts); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete inventory: %w", err)
	}

	return nil
}

// resourceList is the base block used to build the secret containing
// the resources deployed in the cluster.
type resourceList struct {
	Gvk       schema.GroupVersionKind `json:"kind"` //nolint:tagliatelle
	Resources []string                `json:"resources"`
}

// oldResourceList is the v0 of old secret base inventory
type oldResourceList struct {
	Kind      string `json:"kind"`
	Mapping   schema.GroupVersionResource
	Resources []string `json:"resources"`
}

// Resources secrets created with helper/builer version of mlp is incompatible with newer versions
// this function convert old format in the new one
func convertSecretFormat(resources []byte) (map[string]*resourceList, error) {
	oldres := make(map[string]*oldResourceList)
	err := json.Unmarshal(resources, &oldres)
	if err != nil {
		return nil, err
	}

	res := make(map[string]*resourceList)

	for k, v := range oldres {
		res[k] = &resourceList{
			Gvk: schema.GroupVersionKind{
				Group:   v.Mapping.Group,
				Version: v.Mapping.Version,
				Kind:    k,
			},
			Resources: v.Resources}
	}
	return res, nil
}
