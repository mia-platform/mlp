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
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"reflect"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

const (
	miaPlatformPrefix = "mia-platform.eu/"

	deployFilterAnnotation = miaPlatformPrefix + "deploy"
	deployFilterValue      = "once"

	deployChecksumAnnotation = miaPlatformPrefix + "deploy-checksum"

	checksumAnnotation = miaPlatformPrefix + "dependencies-checksum"

	jobGeneratorLabel = miaPlatformPrefix + "autocreate"
	jobGeneratorValue = "true"

	deployAll   = "deploy_all"
	deploySmart = "smart_deploy"
)

var (
	configMapGK = corev1.SchemeGroupVersion.WithKind(reflect.TypeOf(corev1.ConfigMap{}).Name()).GroupKind()
	secretGK    = corev1.SchemeGroupVersion.WithKind(reflect.TypeOf(corev1.Secret{}).Name()).GroupKind()

	deployGK = appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.Deployment{}).Name()).GroupKind()
	dsGK     = appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.DaemonSet{}).Name()).GroupKind()
	stsGK    = appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.StatefulSet{}).Name()).GroupKind()
	podGK    = corev1.SchemeGroupVersion.WithKind(reflect.TypeOf(corev1.Pod{}).Name()).GroupKind()

	validDeployTypeValues = []string{"deploy_all", "smart_deploy"}
)

// podFieldsForGroupKind return the pieces of the path for the pod spec and pod annotations for an unstructured
// object described by gvk. This arrays can be used for retrieving information wihout casting the resource.
func podFieldsForGroupKind(gvk schema.GroupVersionKind) ([]string, []string, error) {
	podSpecFields := []string{}
	annotationsFields := []string{}
	var err error
	switch gvk.GroupKind() {
	case deployGK:
		podSpecFields = []string{"spec", "template", "spec"}
		annotationsFields = []string{"spec", "template", "metadata", "annotations"}
	case dsGK:
		podSpecFields = []string{"spec", "template", "spec"}
		annotationsFields = []string{"spec", "template", "metadata", "annotations"}
	case stsGK:
		podSpecFields = []string{"spec", "template", "spec"}
		annotationsFields = []string{"spec", "template", "metadata", "annotations"}
	case podGK:
		podSpecFields = []string{"spec"}
		annotationsFields = []string{"metadata", "annotations"}
	default:
		apiVersion, kind := gvk.ToAPIVersionAndKind()
		err = fmt.Errorf("unsupported object type for dependencies mutator: \"%s, %s\"", apiVersion, kind)
	}

	return podSpecFields, annotationsFields, err
}

// podSpecFromUnstructured try to extract a podSpec from obj at fields path
func podSpecFromUnstructured(obj *unstructured.Unstructured, fields []string) (corev1.PodSpec, error) {
	var podSpec corev1.PodSpec
	unstrPodSpec, _, err := unstructured.NestedMap(obj.Object, fields...)
	if err != nil {
		return podSpec, err
	}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(unstrPodSpec, &podSpec); err != nil {
		return podSpec, err
	}

	return podSpec, nil
}

// annotationsFromUnstructuredFilds return the annotation present at fields in the obj or an empty map
func annotationsFromUnstructuredFilds(obj *unstructured.Unstructured, fields []string) (map[string]string, error) {
	annotations, _, err := unstructured.NestedStringMap(obj.Object, fields...)
	if err != nil {
		return nil, err
	}

	if annotations == nil {
		annotations = make(map[string]string)
	}

	return annotations, nil
}

// checksumFromData create a Sum512_256 checksum for arbitrary data
func checksumFromData(data interface{}) string {
	encoded, err := yaml.Marshal(data)
	if err != nil {
		return ""
	}

	shasum := sha512.Sum512_256(encoded)
	return hex.EncodeToString(shasum[:])
}
