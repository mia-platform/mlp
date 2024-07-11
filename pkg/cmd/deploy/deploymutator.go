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
	"fmt"

	"github.com/blang/semver/v4"
	dockerref "github.com/distribution/reference"
	"github.com/mia-platform/jpl/pkg/client/cache"
	"github.com/mia-platform/jpl/pkg/mutator"
	"github.com/mia-platform/jpl/pkg/resource"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// deployMutator will add the deploy annotation to workload resources if needed, based on configurations.
//   - if deploy type is 'deploy_all' always set the annotation
//   - if deploy type is smart deploy and forceDeployWhenNoSemver is false set the annotation only if present on
//     remote
//   - if deploy type is smart deploy and forceDeployWhenNoSemver is true, if at least one image in one of its
//     containers don't have a semver tag set the deploy annotation, else set the annotation only if present on
//     remote
type deployMutator struct {
	deployType    string
	forceNoSemver bool
	identifier    string
}

// NewDeployMutator return a new deploy mutator with the given deployment configurations
func NewDeployMutator(deployType string, forceNoSemver bool, deploymentIdentifier string) mutator.Interface {
	return &deployMutator{
		deployType:    deployType,
		forceNoSemver: forceNoSemver,
		identifier:    deploymentIdentifier,
	}
}

// CanHandleResource implement mutator.Interface interface
func (m *deployMutator) CanHandleResource(obj *metav1.PartialObjectMetadata) bool {
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
func (m *deployMutator) Mutate(obj *unstructured.Unstructured, getter cache.RemoteResourceGetter) error {
	podSpecFields, podAnnotationsFields, err := podFieldsForGroupKind(obj.GroupVersionKind())
	if err != nil {
		return err
	}

	addAnnotation := false
	value := ""
	switch m.deployType {
	case deploySmart:
		addAnnotation, value, err = m.smartDeployAnnotation(obj, podSpecFields, podAnnotationsFields, getter)
		if err != nil {
			return err
		}
	case deployAll:
		addAnnotation = true
		value = m.identifier
	}

	if !addAnnotation {
		return nil
	}

	annotations, err := annotationsFromUnstructuredFields(obj, podAnnotationsFields)
	if err != nil {
		return err
	}

	annotations[deployChecksumAnnotation] = value
	return unstructured.SetNestedStringMap(obj.Object, annotations, podAnnotationsFields...)
}

// smartDeployAnnotation return if the object needs a deploy-checksum annotation and the value to set
func (m *deployMutator) smartDeployAnnotation(obj *unstructured.Unstructured, podSpecPath, podAnnotationsFields []string, getter cache.RemoteResourceGetter) (bool, string, error) {
	if m.forceNoSemver {
		noSemverInPod, err := m.checkNoSemverInPod(obj, podSpecPath)
		if noSemverInPod || err != nil {
			return noSemverInPod, m.identifier, err
		}
	}

	// controlla in remoto
	remoteObj, err := getter.Get(context.Background(), resource.ObjectMetadataFromUnstructured(obj))
	if err != nil {
		return false, "", err
	}

	if remoteObj == nil {
		return false, "", nil
	}

	annotations, err := annotationsFromUnstructuredFields(remoteObj, podAnnotationsFields)
	if err != nil {
		return false, "", nil
	}

	value, found := annotations[deployChecksumAnnotation]
	return found, value, nil
}

// checkNoSemverInPod return if at least one container inside a pod spec contains an image without a tag that is
// a semantiv version
func (m *deployMutator) checkNoSemverInPod(obj *unstructured.Unstructured, podSpecFields []string) (bool, error) {
	podSpec, err := podSpecFromUnstructured(obj, podSpecFields)
	if err != nil {
		return false, err
	}

	checkNoSemverTagInPod := func(containers []corev1.Container) (bool, error) {
		for _, container := range containers {
			tag, digest, err := parseImageTag(container.Image)
			if err != nil {
				return false, err
			}

			if len(digest) != 0 {
				return false, nil
			}

			if _, err := semver.ParseTolerant(tag); err != nil {
				return true, nil
			}
		}

		return false, nil
	}

	if found, err := checkNoSemverTagInPod(podSpec.InitContainers); found || err != nil {
		return found, err
	}

	return checkNoSemverTagInPod(podSpec.Containers)
}

// parseImageTag return tag and digest for the given image name string.
// The function will also return "latest" as tag if the name string has no tag defined
func parseImageTag(image string) (string, string, error) {
	named, err := dockerref.ParseNormalizedNamed(image)
	if err != nil {
		return "", "", fmt.Errorf("couldn't parse image name %q: %w", image, err)
	}

	var tag, digest string
	tagged, ok := named.(dockerref.Tagged)
	if ok {
		tag = tagged.Tag()
	}

	digested, ok := named.(dockerref.Digested)
	if ok {
		digest = digested.Digest().String()
	}
	// If no tag was specified, use the default "latest".
	if len(tag) == 0 && len(digest) == 0 {
		tag = "latest"
	}
	return tag, digest, nil
}

// keep it to always check if deployMutator implement correctly the mutator.Interface interface
var _ mutator.Interface = &deployMutator{}
