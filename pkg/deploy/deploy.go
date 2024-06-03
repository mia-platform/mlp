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
	"time"

	"github.com/mia-platform/mlp/internal/utils"
	"github.com/mia-platform/mlp/pkg/resourceutil"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

const (
	dependenciesChecksum = "dependencies-checksum"
	deployAll            = "deploy_all"
)

type k8sClients struct {
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
}

// Run execute the deploy command from cli
func Run(inputPaths []string, deployConfig utils.DeployConfig, opts *utils.Options) {
	restConfig, err := opts.Config.ToRESTConfig()
	utils.CheckError(err, "")

	// The following two options manage client-side throttling to decrease pressure on api-server
	// Kubectl sets 300 bursts 50.0 QPS:
	// https: //github.com/kubernetes/kubectl/blob/master/pkg/cmd/cmd.go#L96
	restConfig.QPS = 100.0
	restConfig.Burst = 500

	clients := &k8sClients{
		dynamic:   dynamic.NewForConfigOrDie(restConfig),
		discovery: discovery.NewDiscoveryClientForConfigOrDie(restConfig),
	}
	currentTime := time.Now()
	err = doRun(clients, opts.Namespace, inputPaths, deployConfig, currentTime)
	utils.CheckError(err, "")
}

func doRun(clients *k8sClients, namespace string, inputPaths []string, deployConfig utils.DeployConfig, currentTime time.Time) error {
	filePaths, err := utils.ExtractYAMLFiles(inputPaths)
	utils.CheckError(err, "Error extracting yaml files")

	resources, err := resourceutil.MakeResources(filePaths, namespace)
	if err != nil {
		fmt.Printf("fails to make resources: %s\n", err)
		return err
	}
	err = prepareResources(deployConfig.DeployType, resources, currentTime)
	if err != nil {
		fmt.Printf("fails to prepare resources: %s\n", err)
		return err
	}

	err = deploy(clients, namespace, resources, deployConfig)
	if err != nil {
		fmt.Printf("fails to deploy: %s", err)
		return err
	}

	return cleanup(clients, namespace, resources)
}

func prepareResources(deployType string, resources []resourceutil.Resource, currentTime time.Time) error {
	configMapMap, secretMap, err := resourceutil.MapSecretAndConfigMap(resources)
	utils.CheckError(err, "error preparing resources")

	for _, res := range resources {
		if res.GroupVersionKind.Kind != deploymentKind && res.GroupVersionKind.Kind != cronJobKind {
			continue
		}
		if deployType == deployAll {
			if err := ensureDeployAll(&res, currentTime); err != nil {
				fmt.Printf("fails to ensure deploy all: %s\n", err)
				return err
			}
		}
		if err := insertDependencies(&res, configMapMap, secretMap); err != nil {
			fmt.Printf("fails to insert dependencies: %s\n", err)
			return err
		}
	}

	return nil
}

func insertDependencies(res *resourceutil.Resource, configMapMap map[string]string, secretMap map[string]string) error {
	var path []string
	var err error
	switch res.GroupVersionKind.Kind {
	case deploymentKind:
		path = []string{"spec", "template", "spec"}
	case cronJobKind:
		path = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	unstrPodSpec, _, err := unstructured.NestedMap(res.Object.Object, path...)
	if err != nil {
		fmt.Printf("fails to get unstructured pod spec %s: %s\n", res.Object.GetName(), err)
		return err
	}
	var podSpec corev1.PodSpec
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstrPodSpec, &podSpec)
	if err != nil {
		fmt.Printf("fails to convert unstructured pod %s spec: %s\n", res.Object.GetName(), err)
		return err
	}
	dependencies := resourceutil.GetPodsDependencies(podSpec)

	// NOTE: ConfigMaps and Secrets that are used as dependency for the resource but do not exist
	// in the resource files provided are ignored and no annotation is created for them.
	var checksumMap = map[string]string{}
	for _, configMapName := range dependencies[resourceutil.ConfigMap] {
		if checksum, ok := configMapMap[configMapName]; ok {
			checksumMap[fmt.Sprintf("%s-configmap", configMapName)] = checksum
		}
	}
	for _, secretName := range dependencies[resourceutil.Secret] {
		if checksum, ok := secretMap[secretName]; ok {
			checksumMap[fmt.Sprintf("%s-secret", secretName)] = checksum
		}
	}

	jsonCheckSumMap, err := json.Marshal(checksumMap)
	if err != nil {
		fmt.Printf("can not convert checksumMap to json: %s", err.Error())
	}

	switch res.GroupVersionKind.Kind {
	case deploymentKind:
		path = []string{"spec", "template", "metadata", "annotations"}
	case cronJobKind:
		path = []string{"spec", "jobTemplate", "spec", "template", "metadata", "annotations"}
	}
	currentAnnotations, found, err := unstructured.NestedStringMap(res.Object.Object,
		path...)
	if err != nil {
		return err
	}
	if !found {
		currentAnnotations = make(map[string]string)
	}
	currentAnnotations[resourceutil.GetMiaAnnotation(dependenciesChecksum)] = string(jsonCheckSumMap)
	return unstructured.SetNestedStringMap(res.Object.Object,
		currentAnnotations,
		path...)
}

// cleanup removes the resources no longer deployed by `mlp` and updates
// the secret in the cluster with the updated set of resources
func cleanup(clients *k8sClients, namespace string, resources []resourceutil.Resource) error {
	actual := makeResourceMap(resources)

	old, err := getOldResourceMap(clients, namespace)
	if err != nil {
		return err
	}

	// Prune only if it is not the first release or after an empty release
	if len(old) != 0 {
		deleteMap := deletedResources(actual, old)

		for _, resourceGroup := range deleteMap {
			err = prune(clients, namespace, resourceGroup)
			if err != nil {
				return err
			}
		}
	}
	err = updateResourceSecret(clients.dynamic, namespace, actual)
	return err
}

func updateResourceSecret(dynamic dynamic.Interface, namespace string, resources map[string]*ResourceList) error {
	secretContent, err := json.Marshal(resources)
	if err != nil {
		return err
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceSecretName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		Data:     map[string][]byte{"resources": secretContent},
	}

	unstr, err := fromRuntimeObjtoUnstruct(secret, secret.GroupVersionKind())
	if err != nil {
		return err
	}

	if _, err = dynamic.Resource(gvrSecrets).
		Namespace(unstr.GetNamespace()).
		Create(context.Background(), unstr, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			_, err = dynamic.Resource(gvrSecrets).
				Namespace(unstr.GetNamespace()).
				Update(context.Background(),
					unstr,
					metav1.UpdateOptions{})
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}
	return nil
}

func prune(clients *k8sClients, namespace string, resourceGroup *ResourceList) error {
	for _, res := range resourceGroup.Resources {
		fmt.Printf("Deleting: %v %v\n", resourceGroup.Gvk.Kind, res)

		gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, *resourceGroup.Gvk)
		if err != nil {
			return err
		}
		onClusterObj, err := clients.dynamic.Resource(gvr).Namespace(namespace).
			Get(context.Background(), res, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				fmt.Printf("already not present on cluster\n")
				continue
			}
			return err
		}
		// delete the object only if the resource has the managed by MIA label
		if onClusterObj.GetLabels()[resourceutil.ManagedByLabel] != resourceutil.ManagedByMia {
			continue
		}
		err = clients.dynamic.Resource(gvr).Namespace(namespace).
			Delete(context.Background(), res, metav1.DeleteOptions{})

		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func deploy(clients *k8sClients, namespace string, resources []resourceutil.Resource, deployConfig utils.DeployConfig) error {
	// for each resource ensure namespace if a namespace is not passed to mlp ensure namespace in the resource, gives error
	// on no namespace passed to mlp and no namespace in yaml
	// The namespace given to mlp overrides yaml namespace
	for _, res := range resources {
		if namespace == "" {
			resourceNamespace := res.Object.GetNamespace()
			if resourceNamespace != "" && deployConfig.EnsureNamespace {
				if err := ensureNamespaceExistence(clients, resourceNamespace); err != nil {
					return err
				}
			} else if resourceNamespace == "" {
				return fmt.Errorf("no namespace passed and no namespace in resource: %s %s", res.GroupVersionKind.Kind, res.Object.GetName())
			}
		} else {
			res.Object.SetNamespace(namespace)
		}
	}

	if namespace != "" && deployConfig.EnsureNamespace {
		if err := ensureNamespaceExistence(clients, namespace); err != nil {
			return err
		}
	}

	// apply the resources
	for _, res := range resources {
		err := apply(clients, res, deployConfig)
		if err != nil {
			return err
		}
	}
	return nil
}

func ensureNamespaceExistence(clients *k8sClients, namespace string) error {
	ns := &unstructured.Unstructured{}
	ns.SetUnstructuredContent(map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]interface{}{
			"name": namespace,
		},
	})

	fmt.Printf("Creating namespace %s\n", namespace)
	if _, err := clients.dynamic.Resource(gvrNamespaces).Create(context.Background(), ns, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return err
	}

	return nil
}
