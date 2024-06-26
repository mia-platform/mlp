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
	"sync"

	"github.com/mia-platform/jpl/pkg/filter"
	"github.com/mia-platform/jpl/pkg/inventory"
	"github.com/mia-platform/jpl/pkg/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
)

// deployOnceFilter will implement a filter that will remove a Secret or ConfigMap if they have a value
// of deployFilterValue in the deployFilterAnnotation and the resource metadata is found in the remote inventory.
// In any other cases the resources are kept.
type deployOnceFilter struct {
	inventory       inventory.Store
	serverObjsCache sets.Set[resource.ObjectMetadata]

	cacheLock sync.Mutex
}

// NewDeployOnceFilter return a new filter for avoiding to apply a resource more than once in its lifetime
func NewDeployOnceFilter(inventory inventory.Store) filter.Interface {
	return &deployOnceFilter{
		inventory: inventory,
	}
}

// Filter implement filter.Interface interface
func (f *deployOnceFilter) Filter(obj *unstructured.Unstructured) (bool, error) {
	objGK := obj.GroupVersionKind().GroupKind()

	switch objGK {
	case configMapGK, secretGK:
		annotations := obj.GetAnnotations()
		if annotations == nil {
			return false, nil
		}

		if value, found := annotations[deployFilterAnnotation]; !found || value != deployFilterValue {
			return false, nil
		}
	default:
		return false, nil
	}

	f.cacheLock.Lock()
	defer f.cacheLock.Unlock()

	if err := f.populateCache(context.TODO()); err != nil {
		return false, err
	}

	return f.serverObjsCache.Has(resource.ObjectMetadataFromUnstructured(obj)), nil
}

// populateCache will call once the remote server for retrieving the actual inventory, and then return always the
// loaded cache
func (f *deployOnceFilter) populateCache(ctx context.Context) error {
	if f.serverObjsCache != nil {
		return nil
	}

	set, err := f.inventory.Load(ctx)
	if err != nil {
		return err
	}

	f.serverObjsCache = set
	return nil
}

// keep it to always check if deployOnceFilter implement correctly the filter.Interface interface
var _ filter.Interface = &deployOnceFilter{}
