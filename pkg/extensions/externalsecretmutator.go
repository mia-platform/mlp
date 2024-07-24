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
	"maps"
	"slices"

	extsecv1beta1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
	"github.com/mia-platform/jpl/pkg/client/cache"
	"github.com/mia-platform/jpl/pkg/mutator"
	"github.com/mia-platform/jpl/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
)

type externalSecretsMutator struct {
	externalSecretMap map[string]*unstructured.Unstructured
	secretsStores     map[string]*unstructured.Unstructured
}

func NewExternalSecretsMutator(objects []*unstructured.Unstructured) mutator.Interface {
	externalSecretMap := make(map[string]*unstructured.Unstructured)
	secretsStores := make(map[string]*unstructured.Unstructured)

	for _, obj := range objects {
		switch obj.GroupVersionKind().GroupKind() {
		case extsecGK:
			maps.Copy(externalSecretMap, secretForExternalSecret(obj))
		case extSecStoreGK:
			secretsStores[externalSecretStoreKey(obj.GroupVersionKind().Kind, obj.GetName(), obj.GetNamespace())] = obj
		}
	}

	return &externalSecretsMutator{
		externalSecretMap: externalSecretMap,
		secretsStores:     secretsStores,
	}
}

// CanHandleResource implement mutator.Interface interface
func (m *externalSecretsMutator) CanHandleResource(obj *metav1.PartialObjectMetadata) bool {
	if len(m.externalSecretMap) == 0 {
		return false
	}

	switch obj.GroupVersionKind().GroupKind() {
	case deployGK:
		return true
	case dsGK:
		return true
	case stsGK:
		return true
	case podGK:
		return true
	case extsecGK:
		return true
	}

	return false
}

// Mutate implement mutator.Interface interface
func (m *externalSecretsMutator) Mutate(obj *unstructured.Unstructured, _ cache.RemoteResourceGetter) error {
	if obj.GroupVersionKind().GroupKind() == extsecGK {
		return m.annotateExternalSecret(obj)
	}

	podSpecFields, _, err := podFieldsForGroupKind(obj.GroupVersionKind())
	if err != nil {
		return err
	}

	podSpec, err := podSpecFromUnstructured(obj, podSpecFields)
	if err != nil {
		return err
	}

	externalSecrets := m.externalSecretsForPodSpec(podSpec, obj.GetNamespace())
	if len(externalSecrets) == 0 {
		return nil
	}

	return resource.SetObjectExplicitDependencies(obj, externalSecrets)
}

// keep it to always check if dependenciesMutator implement correctly the mutator.Interface interface
var _ mutator.Interface = &dependenciesMutator{}

// secretForExternalSecret return a map of secret name and external secret resources
func secretForExternalSecret(obj *unstructured.Unstructured) map[string]*unstructured.Unstructured {
	externalSecrets := make(map[string]*unstructured.Unstructured)

	extsec := new(extsecv1beta1.ExternalSecret)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, extsec); err != nil {
		return externalSecrets
	}

	secretName := extsec.Spec.Target.Name
	if len(secretName) == 0 {
		secretName = extsec.ObjectMeta.Name
	}
	externalSecrets[secretKey(secretName, obj.GetNamespace())] = obj

	return externalSecrets
}

func secretKey(name, namespace string) string {
	return name + ":" + namespace
}

// externalSecretStoreKey return a key rappresentation for a secret store given its kind, name and eventual namespace
func externalSecretStoreKey(kind, name, namespace string) string {
	if len(kind) == 0 {
		kind = extsecv1beta1.SecretStoreKind
	}

	return kind + ":" + name + ":" + namespace
}

// annotateExternalSecret mutate the ExternalSecret obj with the depends-on annotation with all the stores
// referrend inside it.
func (m *externalSecretsMutator) annotateExternalSecret(obj *unstructured.Unstructured) error {
	extsec := new(extsecv1beta1.ExternalSecret)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, extsec); err != nil {
		return err
	}
	stores := make(sets.Set[resource.ObjectMetadata])

	// check if there is a default secret store set for all external secert
	if len(extsec.Spec.SecretStoreRef.Name) != 0 {
		storeKey := externalSecretStoreKey(extsec.Spec.SecretStoreRef.Kind, extsec.Spec.SecretStoreRef.Name, extsec.Namespace)
		if obj, found := m.secretsStores[storeKey]; found {
			stores.Insert(resource.ObjectMetadataFromUnstructured(obj))
		}
	}

	extractStoreFromSourceRef := func(sourceRef *extsecv1beta1.SecretStoreRef) (*unstructured.Unstructured, bool) {
		if sourceRef == nil || len(sourceRef.Name) == 0 {
			return nil, false
		}
		storeKey := externalSecretStoreKey(sourceRef.Kind, sourceRef.Name, extsec.Namespace)
		obj, found := m.secretsStores[storeKey]
		return obj, found
	}

	// check if there are specific stores for every data
	for _, data := range extsec.Spec.Data {
		if data.SourceRef == nil {
			continue
		}
		if obj, found := extractStoreFromSourceRef(data.SourceRef.SecretStoreRef); found {
			stores.Insert(resource.ObjectMetadataFromUnstructured(obj))
		}
	}

	// check if there are specific stores for every dataFrom
	for _, dataFrom := range extsec.Spec.DataFrom {
		if dataFrom.SourceRef == nil {
			continue
		}
		if obj, found := extractStoreFromSourceRef(dataFrom.SourceRef.SecretStoreRef); found {
			stores.Insert(resource.ObjectMetadataFromUnstructured(obj))
		}
	}

	return resource.SetObjectExplicitDependencies(obj, stores.UnsortedList())
}

func (m *externalSecretsMutator) externalSecretsForPodSpec(pod corev1.PodSpec, namespace string) []resource.ObjectMetadata {
	secretsReferences := make([]string, 0)
	for _, volume := range pod.Volumes {
		fromSecret := volume.Secret
		if fromSecret != nil {
			secretsReferences = append(secretsReferences, secretKey(fromSecret.SecretName, namespace))
			continue
		}
	}

	fromContainers := func(containers []corev1.Container) {
		for _, container := range containers {
			for _, env := range container.Env {
				if env.ValueFrom == nil {
					continue
				}

				if env.ValueFrom.SecretKeyRef != nil {
					name := env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
					secretsReferences = append(secretsReferences, secretKey(name, namespace))
				}
			}
		}
	}

	fromContainers(pod.InitContainers)
	fromContainers(pod.Containers)

	externalSecrets := make([]resource.ObjectMetadata, 0)
	for _, key := range secretsReferences {
		if externalSecret, found := m.externalSecretMap[key]; found {
			objMeta := resource.ObjectMetadataFromUnstructured(externalSecret)
			if !slices.Contains(externalSecrets, objMeta) {
				externalSecrets = append(externalSecrets, objMeta)
			}
		}
	}

	return externalSecrets
}
