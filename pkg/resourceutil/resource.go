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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"github.com/Masterminds/semver"
	appsv1 "k8s.io/api/apps/v1"
	batchapiv1beta1 "k8s.io/api/batch/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/yaml"
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
	Filepath  string
	Name      string
	Head      ResourceHead
	Namespace string
	Info      *resource.Info
}

// ResourceHead the head of the resource
type ResourceHead struct {
	GroupVersion string `json:"apiVersion"`
	Kind         string `json:"kind,omitempty"`
	Metadata     *struct {
		Name        string            `json:"name"`
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata,omitempty"`
}

var accessor = meta.NewAccessor()

// NewResource create a new Resource from a file at `filepath`
// does NOT support multiple documents inside a single file
func NewResource(filepath string) (*Resource, error) {

	data, err := utils.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	var head ResourceHead
	if err = yaml.Unmarshal([]byte(data), &head); err != nil {
		return nil, err
	}

	return &Resource{
		Filepath: filepath,
		Name:     head.Metadata.Name,
		Head:     head,
	}, nil
}

// MakeInfo is the default function used to build `resource.Info`. It uses a builder to create
// the Infos starting from a YAML file path and then it set the correct namespace to the resource.
func MakeInfo(builder InfoGenerator, namespace string, path string) (*resource.Info, error) {
	infos, err := builder.FromFile(path)
	if err != nil {
		return nil, err
	}

	if len(infos) != 1 {
		return nil, errors.New("Multiple objects in single yaml file currently not supported")
	}

	info := infos[0]
	info.Namespace = namespace
	return info, nil
}

// MakeResources creates a resource list and sorts them according to
// the standard ordering strategy
func MakeResources(opts *utils.Options, filePaths []string) ([]Resource, error) {

	resources := []Resource{}
	for _, path := range filePaths {

		// built every time because there is no mapping between `resourceutil.Resource`
		// and its corresponding `resource.Info`
		builder := NewBuilder(opts.Config)

		res, err := MakeResource(builder, opts.Namespace, path)

		if err != nil {
			return nil, err
		}

		resources = append(resources, *res)
	}

	resources = SortResourcesByKind(resources, nil)
	return resources, nil
}

// MakeResource creates a `Resource` from file
func MakeResource(infoGen InfoGenerator, namespace, path string) (*Resource, error) {
	res, err := NewResource(path)
	if err != nil {
		return nil, err
	}
	res.Namespace = namespace
	info, err := MakeInfo(infoGen, res.Namespace, path)
	if err != nil {
		return nil, err
	}

	accessor.SetNamespace(info.Object, "")

	err = updateLabels(info.Object, map[string]string{
		ManagedByLabel: ManagedByMia,
	})

	if err != nil {
		return nil, err
	}

	res.Info = info
	return res, nil
}

// updateMapStringChecksum add for each key of the input map a key equals to "name:key" and a values as its value checksum
func updateMapStringChecksum(inputMap map[string]string, name string, mapToModified map[string]string) {
	for key, value := range inputMap {
		mapToModified[fmt.Sprintf("%s-%s", name, key)] = GetChecksum([]byte(value))
	}
}

// updateMapByteChecksum add for each key of the input map a key equals to "name:key" and a values as its value checksum
func updateMapByteChecksum(inputMap map[string][]byte, name string, mapToModified map[string]string) {
	for key, value := range inputMap {
		mapToModified[fmt.Sprintf("%s-%s", name, key)] = GetChecksum(value)
	}
}

// MapSecretAndConfigMap returns two mappings, one for Secret and one for ConfigMap:
// each map holds the resource name as the key and its checksum as the value
func MapSecretAndConfigMap(resources []Resource) (map[string]string, map[string]string, error) {
	var configmaps = map[string]string{}
	var secrets = map[string]string{}

	for _, res := range resources {
		switch res.Head.Kind {
		case ConfigMap:
			allContent, err := json.Marshal(res.Info.Object)
			if err != nil {
				return nil, nil, err
			}
			configmaps[res.Name] = GetChecksum(allContent)

			configMap, err := GetApiv1ConfigMapFromObject(res.Info.Object)
			if err != nil {
				return nil, nil, err
			}

			updateMapStringChecksum(configMap.Data, res.Name, configmaps)
			updateMapByteChecksum(configMap.BinaryData, res.Name, configmaps)
		case Secret:
			allContent, err := json.Marshal(res.Info.Object)
			if err != nil {
				return nil, nil, err
			}
			secrets[res.Name] = GetChecksum(allContent)

			secret, err := GetApiv1SecretFromObject(res.Info.Object)
			if err != nil {
				return nil, nil, err
			}
			updateMapStringChecksum(secret.StringData, res.Name, secrets)
			updateMapByteChecksum(secret.Data, res.Name, secrets)
		}
	}
	return configmaps, secrets, nil
}

// GetPodsDependencies returns a map where keys are Secret and ConfigMap and each key has a list of dependencies of that kind
func GetPodsDependencies(podSpec apiv1.PodSpec) map[string][]string {
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

// updateLabels add or update the current object labels with
// the ones contained in `new` map.
func updateLabels(obj runtime.Object, new map[string]string) error {

	current, err := accessor.Labels(obj)

	if err != nil {
		return err
	}

	result := mergeLabels(current, new)

	return accessor.SetLabels(obj, result)
}

func mergeLabels(current, new map[string]string) map[string]string {
	result := make(map[string]string)

	for k, v := range current {
		result[k] = v
	}

	for k, v := range new {
		result[k] = v
	}

	return result
}

func getKeysFromMap(mapStringBool map[string]bool) []string {
	keys := []string{}
	for key := range mapStringBool {
		keys = append(keys, key)
	}
	return keys
}

func contains(stringSlice []string, str string) bool {
	for _, value := range stringSlice {
		if value == str {
			return true
		}
	}
	return false
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

// GetAppsv1DeploymentFromObject is used to get an appsv1.Deployment from runtime.Object
func GetAppsv1DeploymentFromObject(obj runtime.Object) (*appsv1.Deployment, error) {
	yamlObject, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("resource info object can not be marshalled")
	}

	var deployment appsv1.Deployment
	if err := yaml.UnmarshalStrict(yamlObject, &deployment); err != nil {
		return nil, fmt.Errorf("not a valid deployment")
	}
	return &deployment, nil
}

// GetBatchapiv1beta1CronJobFromObject is used to get a batchapiv1beta1.CronJob from runtime.Object
func GetBatchapiv1beta1CronJobFromObject(obj runtime.Object) (*batchapiv1beta1.CronJob, error) {
	yamlObject, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("resource info object can not be marshalled")
	}

	var cronJob batchapiv1beta1.CronJob
	if err := yaml.UnmarshalStrict(yamlObject, &cronJob); err != nil {
		return nil, fmt.Errorf("not a valid cronJob")
	}
	return &cronJob, nil
}

// GetApiv1ConfigMapFromObject is used to get an apiv1.ConfigMap from runtime.Object
func GetApiv1ConfigMapFromObject(obj runtime.Object) (*apiv1.ConfigMap, error) {
	yamlObject, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("resource info object can not be marshalled")
	}

	var configMap apiv1.ConfigMap
	if err := yaml.UnmarshalStrict(yamlObject, &configMap); err != nil {
		return nil, fmt.Errorf("not a valid ConfigMap")
	}
	return &configMap, nil
}

// GetApiv1SecretFromObject is used to get an apiv1.Secret from runtime.Object
func GetApiv1SecretFromObject(obj runtime.Object) (*apiv1.Secret, error) {
	yamlObject, err := yaml.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("resource info object can not be marshalled")
	}

	var secret apiv1.Secret
	if err := yaml.UnmarshalStrict(yamlObject, &secret); err != nil {
		return nil, fmt.Errorf("not a valid Secret")
	}
	return &secret, nil
}

//GetContainer get a container
func GetContainer(image string) v1.Container {
	container := v1.Container{}

	if image != "" {
		container.Image = image
	}

	return container
}

//GetPodSpec get a podSpec
func GetPodSpec(volumes *[]v1.Volume, containers *[]v1.Container) v1.PodSpec {
	podSpec := v1.PodSpec{}

	if volumes != nil {
		podSpec.Volumes = *volumes
	}

	if containers != nil {
		podSpec.Containers = *containers
	}
	return podSpec
}

//GetSecretResource get a Secret Resource
func GetSecretResource(res *Resource, secretType *v1.SecretType) *resource.Info {
	secret := apiv1.Secret{}
	secret.TypeMeta = metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"}
	secret.ObjectMeta.Name = res.Name
	secret.ObjectMeta.Namespace = res.Namespace

	if secretType != nil {
		secret.Type = *secretType
	}

	return &resource.Info{
		Object:    &secret,
		Namespace: res.Namespace,
		Name:      res.Name,
	}
}

//GetDeploymentResource get a Deployment resource
func GetDeploymentResource(res *Resource, annotations map[string]string, podSpec *v1.PodSpec) *resource.Info {
	deployment := appsv1.Deployment{}
	deployment.TypeMeta = metav1.TypeMeta{
		Kind:       "Deployment",
		APIVersion: "apps/v1",
	}
	deployment.ObjectMeta.Name = res.Name
	deployment.ObjectMeta.Namespace = res.Namespace

	if annotations != nil {
		deployment.Spec.Template.ObjectMeta.Annotations = annotations
	}
	if podSpec != nil {
		deployment.Spec.Template.Spec = *podSpec
	}

	return &resource.Info{
		Object:    &deployment,
		Namespace: res.Namespace,
		Name:      res.Name,
		Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}},
	}
}

//GetCronJobResource get a cronJob resource
func GetCronJobResource(res *Resource, annotations map[string]string, podSpec *v1.PodSpec) *resource.Info {
	cronJob := batchapiv1beta1.CronJob{}
	cronJob.TypeMeta = metav1.TypeMeta{
		Kind:       "CronJob",
		APIVersion: "batch/v1beta1",
	}
	cronJob.ObjectMeta.Name = res.Name
	cronJob.ObjectMeta.Namespace = res.Namespace

	if annotations != nil {
		cronJob.Spec.JobTemplate.Spec.Template.ObjectMeta.Annotations = annotations
	}
	if podSpec != nil {
		cronJob.Spec.JobTemplate.Spec.Template.Spec = *podSpec
	}

	return &resource.Info{
		Object:    &cronJob,
		Namespace: res.Namespace,
		Name:      res.Name,
		Mapping:   &meta.RESTMapping{GroupVersionKind: schema.GroupVersionKind{Group: "batch", Version: "v1beta1", Kind: "CronJob"}},
	}
}

// IsNotUsingSemver is used to check if a resoure is following semver or not
func IsNotUsingSemver(target *Resource) (bool, error) {
	var containers []apiv1.Container
	switch target.Head.Kind {
	case "Deployment":
		desiredDeployment, err := GetAppsv1DeploymentFromObject(target.Info.Object)
		if err != nil {
			return false, fmt.Errorf("Resource %s: %s", target.Name, err.Error())
		}

		containers = desiredDeployment.Spec.Template.Spec.Containers
	case "CronJob":
		desiredCronJob, err := GetBatchapiv1beta1CronJobFromObject(target.Info.Object)
		if err != nil {
			return false, fmt.Errorf("Resource %s: %s", target.Name, err.Error())
		}

		containers = desiredCronJob.Spec.JobTemplate.Spec.Template.Spec.Containers
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
