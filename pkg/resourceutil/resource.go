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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/mia-platform/mlp/internal/utils"
	appsv1 "k8s.io/api/apps/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/restmapper"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	// ManagedByLabel is the label used to identify the agent
	// responsible of deploying the resources into the cluster.
	ManagedByLabel = "app.kubernetes.io/managed-by"
	// ManagedByMia is used to identify the resources deployed
	// with `mlp`.
	ManagedByMia = "mia-platform"
	// ConfigMap is the resource kind for ConfigMaps.
	ConfigMap = "ConfigMap"
	// Secret is the resource kind for Secrets.
	Secret = "Secret"
)

// Resource a resource reppresentation
type Resource struct {
	Filepath         string
	GroupVersionKind *schema.GroupVersionKind
	Object           unstructured.Unstructured
}

func FromGVKtoGVR(discoveryClient discovery.DiscoveryInterface, gvk schema.GroupVersionKind) (schema.GroupVersionResource, error) {
	mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(discoveryClient))
	a, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
	if err != nil {
		return schema.GroupVersionResource{}, err
	}
	return a.Resource, nil
}

// NewResources creates new Resources from a file at `filepath`
// support multiple documents inside a single file
func NewResources(filepath, namespace string) ([]Resource, error) {
	resources := make([]Resource, 0)
	var stream []byte
	var err error

	if filepath == utils.StdinToken {
		stream, err = io.ReadAll(os.Stdin)
	} else {
		stream, err = utils.ReadFile(filepath)
	}
	if err != nil {
		return nil, err
	}

	// split resources on --- yaml document delimiter
	re := regexp.MustCompile(`\n---\n`)
	for _, resourceYAML := range re.Split(string(stream), -1) {
		if len(resourceYAML) == 0 {
			continue
		}

		u := unstructured.Unstructured{Object: map[string]interface{}{}}
		if err := k8syaml.Unmarshal([]byte(resourceYAML), &u.Object); err != nil {
			return nil, fmt.Errorf("resource %s: %s", filepath, err)
		}
		gvk := u.GroupVersionKind()
		u.SetNamespace(namespace)

		resources = append(resources,
			Resource{
				Filepath:         filepath,
				GroupVersionKind: &gvk,
				Object:           u,
			})
	}
	return resources, nil
}

// MakeResources creates a resource list and sorts them according to
// the standard ordering strategy
func MakeResources(filePaths []string, namespace string) ([]Resource, error) {
	resources := []Resource{}
	for _, path := range filePaths {
		res, err := NewResources(path, namespace)
		if err != nil {
			return nil, err
		}
		res = AddManagedByMiaLabel(res)
		resources = append(resources, res...)
	}

	resources = SortResourcesByKind(resources, nil)
	return resources, nil
}

func AddManagedByMiaLabel(resources []Resource) []Resource {
	for _, res := range resources {
		origLabel := res.Object.GetLabels()
		if origLabel == nil {
			origLabel = make(map[string]string)
		}
		origLabel[ManagedByLabel] = ManagedByMia
		res.Object.SetLabels(origLabel)
	}
	return resources
}

// updateMapStringChecksum add for each key of the input map a key equals to "name:key" and a values as its value checksum
func updateMapStringChecksum(inputMap map[string]string, name string, mapToModified map[string]string) {
	for key, value := range inputMap {
		mapToModified[fmt.Sprintf("%s-%s", name, key)] = GetChecksum([]byte(value))
	}
}

// MapSecretAndConfigMap returns two mappings, one for Secret and one for ConfigMap:
// each map holds the resource name as the key and its checksum as the value
func MapSecretAndConfigMap(resources []Resource) (map[string]string, map[string]string, error) {
	var configmaps = map[string]string{}
	var secrets = map[string]string{}

	for _, res := range resources {
		switch res.GroupVersionKind.Kind {
		case ConfigMap:
			var cm corev1.ConfigMap
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object.Object, &cm)
			if err != nil {
				return nil, nil, err
			}
			allContent, err := json.Marshal(cm)
			if err != nil {
				return nil, nil, err
			}

			configmaps[res.Object.GetName()] = GetChecksum(allContent)

			cmData, _, err := unstructured.NestedStringMap(res.Object.Object, "data")
			if err != nil {
				return nil, nil, err
			}
			cmBinData, _, err := unstructured.NestedStringMap(res.Object.Object, "binaryData")
			if err != nil {
				return nil, nil, err
			}
			updateMapStringChecksum(cmData, res.Object.GetName(), configmaps)
			updateMapStringChecksum(cmBinData, res.Object.GetName(), configmaps)
		case Secret:
			var sec corev1.ConfigMap
			err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object.Object, &sec)
			if err != nil {
				return nil, nil, err
			}

			allContent, err := json.Marshal(sec)
			if err != nil {
				return nil, nil, err
			}
			secrets[res.Object.GetName()] = GetChecksum(allContent)

			secData, _, err := unstructured.NestedStringMap(res.Object.Object, "stringData")
			if err != nil {
				return nil, nil, err
			}
			secBinData, _, err := unstructured.NestedStringMap(res.Object.Object, "data")
			if err != nil {
				return nil, nil, err
			}
			updateMapStringChecksum(secData, res.Object.GetName(), secrets)
			updateMapStringChecksum(secBinData, res.Object.GetName(), secrets)
		}
	}
	return configmaps, secrets, nil
}

// GetPodsDependencies returns a map where keys are Secret and ConfigMap and each key has a list of dependencies of that kind
func GetPodsDependencies(podSpec corev1.PodSpec) map[string][]string {
	secretsDependencies := make(map[string]bool)
	configMapsDependencies := make(map[string]bool)

	for _, volume := range podSpec.Volumes {
		if volume.Secret != nil {
			secretsDependencies[volume.Secret.SecretName] = true
		}

		if volume.ConfigMap != nil {
			configMapsDependencies[volume.ConfigMap.LocalObjectReference.Name] = true
		}
	}

	for _, container := range podSpec.Containers {
		for _, env := range container.Env {
			if env.ValueFrom == nil {
				continue
			}

			if env.ValueFrom.SecretKeyRef != nil {
				secretMetaName := env.ValueFrom.SecretKeyRef.LocalObjectReference.Name
				secretKey := env.ValueFrom.SecretKeyRef.Key

				if _, ok := secretsDependencies[secretMetaName]; !ok {
					if secretKey == "" {
						secretsDependencies[secretMetaName] = true
					} else {
						secretsDependencies[fmt.Sprintf("%s-%s", secretMetaName, secretKey)] = true
					}
				}
			}

			if env.ValueFrom.ConfigMapKeyRef != nil {
				configMapMetaName := env.ValueFrom.ConfigMapKeyRef.LocalObjectReference.Name
				configMapKey := env.ValueFrom.ConfigMapKeyRef.Key

				if _, ok := configMapsDependencies[configMapMetaName]; !ok {
					if configMapKey == "" {
						configMapsDependencies[configMapMetaName] = true
					} else {
						configMapsDependencies[fmt.Sprintf("%s-%s", configMapMetaName, configMapKey)] = true
					}
				}
			}
		}
	}

	var dependencies = map[string][]string{}

	dependencies[Secret] = getKeysFromMap(secretsDependencies)
	dependencies[ConfigMap] = getKeysFromMap(configMapsDependencies)
	return dependencies
}

func getKeysFromMap(mapStringBool map[string]bool) []string {
	keys := []string{}
	for key := range mapStringBool {
		keys = append(keys, key)
	}
	return keys
}

// GetChecksum is used to calculate a checksum using an array of bytes
func GetChecksum(content []byte) string {
	checkSum := sha256.Sum256(content)
	return hex.EncodeToString(checkSum[:])
}

// GetMiaAnnotation is used to get an annotation name following a pattern used in mia-platform
func GetMiaAnnotation(name string) string {
	return fmt.Sprintf("mia-platform.eu/%s", strings.ReplaceAll(name, " ", "-"))
}

// GetPodSpec get a podSpec
func GetPodSpec(volumes *[]corev1.Volume, containers *[]corev1.Container) corev1.PodSpec {
	podSpec := corev1.PodSpec{}

	if volumes != nil {
		podSpec.Volumes = *volumes
	}

	if containers != nil {
		podSpec.Containers = *containers
	}
	return podSpec
}

// IsNotUsingSemver is used to check if a resoure is following semver or not
func IsNotUsingSemver(target *Resource) (bool, error) {
	var containers []corev1.Container
	var err error
	switch target.GroupVersionKind.Kind {
	case "Deployment":
		var desiredDeployment appsv1.Deployment
		err = runtime.DefaultUnstructuredConverter.
			FromUnstructured(target.Object.Object, &desiredDeployment)
		containers = desiredDeployment.Spec.Template.Spec.Containers
	case "CronJob":
		var desiredCronJob batchv1beta1.CronJob
		err = runtime.DefaultUnstructuredConverter.
			FromUnstructured(target.Object.Object, &desiredCronJob)
		containers = desiredCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers
	}
	if err != nil {
		return false, err
	}

	for _, container := range containers {
		if !strings.Contains(container.Image, ":") {
			return true, nil
		}
		imageVersion := strings.Split(container.Image, ":")[1]
		if _, err := semver.NewVersion(imageVersion); err != nil {
			return true, nil
		}
	}
	return false, nil
}
