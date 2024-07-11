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
	"maps"

	"github.com/mia-platform/jpl/pkg/client/cache"
	"github.com/mia-platform/jpl/pkg/mutator"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

// dependenciesMutator will add annotations with hashed content of mounted config map or secrets to force
// updates and redeploy in case the content is changed
type dependenciesMutator struct {
	checksumsMap map[string]string
}

// NewDependenciesMutator return a new mutator using ConfigMaps and Secrets found in objects
func NewDependenciesMutator(objects []*unstructured.Unstructured) mutator.Interface {
	checksumsMap := make(map[string]string)

	for _, obj := range objects {
		switch obj.GroupVersionKind().GroupKind() {
		case configMapGK:
			maps.Copy(checksumsMap, checksumsFromConfigMap(obj))
		case secretGK:
			maps.Copy(checksumsMap, checksumsFromSecret(obj))
		}
	}

	return &dependenciesMutator{
		checksumsMap: checksumsMap,
	}
}

// CanHandleResource implement mutator.Interface interface
func (m *dependenciesMutator) CanHandleResource(obj *metav1.PartialObjectMetadata) bool {
	if len(m.checksumsMap) == 0 {
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
	}

	return false
}

// Mutate implement mutator.Interface interface
func (m *dependenciesMutator) Mutate(obj *unstructured.Unstructured, _ cache.RemoteResourceGetter) error {
	podSpecFields, podAnnotationsFields, err := podFieldsForGroupKind(obj.GroupVersionKind())
	if err != nil {
		return err
	}

	podSpec, err := podSpecFromUnstructured(obj, podSpecFields)
	if err != nil {
		return err
	}

	checksums := m.checksumsForPodSpec(podSpec, obj.GetNamespace())
	if len(checksums) == 0 {
		return nil
	}

	annotations, err := annotationsFromUnstructuredFields(obj, podAnnotationsFields)
	if err != nil {
		return err
	}

	annotations[checksumAnnotation] = checksumFromData(checksums)
	return unstructured.SetNestedStringMap(obj.Object, annotations, podAnnotationsFields...)
}

// keep it to always check if dependenciesMutator implement correctly the mutator.Interface interface
var _ mutator.Interface = &dependenciesMutator{}

// checksumsFromConfigMap return a map of checksums, containing the full value of the configmap,
// and single checksums for every data and binaryData present in the configmap.
// The keys are the configmap kind, name, namespace and key name if necessary.
func checksumsFromConfigMap(obj *unstructured.Unstructured) map[string]string {
	checksums := make(map[string]string)
	cmKey := configMapGK.Kind + obj.GetName() + obj.GetNamespace()

	cm := new(corev1.ConfigMap)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, cm); err != nil {
		return checksums
	}

	totalData := make(map[string][]byte)
	maps.Copy(totalData, cm.BinaryData)
	for key, value := range cm.Data {
		checksums[cmKey+key] = checksumFromData(value)
		totalData[key] = []byte(value)
	}

	for key, value := range cm.BinaryData {
		checksums[cmKey+key] = checksumFromData(value)
	}

	checksums[cmKey] = checksumFromData(totalData)
	return checksums
}

// checksumsFromSecret return a map of checksums, containing the full value of the secret,
// and single checksums for every data and stringData present in the configmap.
// The keys are the secret kind, name, namespace and key name if necessary.
func checksumsFromSecret(obj *unstructured.Unstructured) map[string]string {
	checksums := make(map[string]string)
	secKey := secretGK.Kind + obj.GetName() + obj.GetNamespace()

	sec := new(corev1.Secret)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, sec); err != nil {
		return checksums
	}

	totalData := make(map[string][]byte)
	maps.Copy(totalData, sec.Data)
	for key, value := range sec.Data {
		checksums[secKey+key] = checksumFromData(value)
	}

	for key, value := range sec.StringData {
		checksums[secKey+key] = checksumFromData(value)
		totalData[key] = []byte(value)
	}

	checksums[secKey] = checksumFromData(totalData)

	return checksums
}

// checksumsForPodSpec
func (m *dependenciesMutator) checksumsForPodSpec(pod corev1.PodSpec, namespace string) map[string]string {
	dependencies := make([]string, 0)
	cmKind := configMapGK.Kind
	secKind := secretGK.Kind

	for _, volume := range pod.Volumes {
		fromSecret := volume.Secret
		if fromSecret != nil {
			dependencies = append(dependencies, secKind+fromSecret.SecretName+namespace)
			continue
		}

		fromConfigMap := volume.ConfigMap
		if fromConfigMap != nil {
			dependencies = append(dependencies, cmKind+fromConfigMap.Name+namespace)
			continue
		}
	}

	fromContainers := func(containers []corev1.Container) {
		for _, container := range containers {
			for _, env := range container.Env {
				if env.ValueFrom == nil {
					continue
				}

				if env.ValueFrom.ConfigMapKeyRef != nil {
					cmName := env.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name
					key := env.ValueFrom.ConfigMapKeyRef.Key
					dependencies = append(dependencies, cmKind+cmName+namespace+key)
					continue
				}

				if env.ValueFrom.SecretKeyRef != nil {
					secName := env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
					key := env.ValueFrom.SecretKeyRef.Key
					dependencies = append(dependencies, secKind+secName+namespace+key)
				}
			}
		}
	}

	fromContainers(pod.InitContainers)
	fromContainers(pod.Containers)

	checksums := make(map[string]string)
	for _, key := range dependencies {
		if shasum, found := m.checksumsMap[key]; found {
			checksums[key] = shasum
		}
	}

	return checksums
}
