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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mia-platform/mlp/internal/utils"
	"github.com/mia-platform/mlp/pkg/resourceutil"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/scheme"
)

const (
	dependenciesChecksum      = "dependencies-checksum"
	deployChecksum            = "deploy-checksum"
	smartDeploy               = "smart_deploy"
	deployAll                 = "deploy_all"
	awaitCompletionAnnotation = "mia-platform.eu/await-completion"
)

type k8sClients struct {
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
}

// Run execute the deploy command from cli
func Run(inputPaths []string, deployConfig utils.DeployConfig, opts *utils.Options) {
	restConfig, err := opts.Config.ToRESTConfig()
	utils.CheckError(err)

	// The following two options manage client-side throttling to decrease pressure on api-server
	// Kubectl sets 300 bursts 50.0 QPS:
	// https://github.com/kubernetes/kubectl/blob/0862c57c87184432986c85674a237737dabc53fa/pkg/cmd/cmd.go#L92
	restConfig.QPS = 500.0
	restConfig.Burst = 500

	clients := &k8sClients{
		dynamic:   dynamic.NewForConfigOrDie(restConfig),
		discovery: discovery.NewDiscoveryClientForConfigOrDie(restConfig),
	}
	currentTime := time.Now()
	err = doRun(clients, opts.Namespace, inputPaths, deployConfig, currentTime)
	utils.CheckError(err)
}

func doRun(clients *k8sClients, namespace string, inputPaths []string, deployConfig utils.DeployConfig, currentTime time.Time) error {
	filePaths, err := utils.ExtractYAMLFiles(inputPaths)
	utils.CheckError(err)

	resources, err := resourceutil.MakeResources(filePaths, namespace)
	if err != nil {
		return err
	}
	err = prepareResources(deployConfig.DeployType, resources, currentTime)
	if err != nil {
		return err
	}

	err = deploy(clients, namespace, resources, deployConfig)
	if err != nil {
		return err
	}

	return cleanup(clients, namespace, resources)
}

func prepareResources(deployType string, resources []resourceutil.Resource, currentTime time.Time) error {
	configMapMap, secretMap, err := resourceutil.MapSecretAndConfigMap(resources)
	utils.CheckError(err)

	for _, res := range resources {
		if res.GroupVersionKind.Kind != "Deployment" && res.GroupVersionKind.Kind != "CronJob" {
			continue
		}
		if deployType == deployAll {
			if err := ensureDeployAll(&res, currentTime); err != nil {
				return err
			}
		}
		if err := insertDependencies(&res, configMapMap, secretMap); err != nil {
			return err
		}
	}

	return nil
}

func insertDependencies(res *resourceutil.Resource, configMapMap map[string]string, secretMap map[string]string) error {
	var dependencies = map[string][]string{}
	var path []string
	var err error
	switch res.GroupVersionKind.Kind {
	case "Deployment":
		path = []string{"spec", "template", "spec"}
	case "CronJob":
		path = []string{"spec", "jobTemplate", "spec", "template", "spec"}
	}
	unstrPodSpec, _, err := unstructured.NestedMap(res.Object.Object, path...)
	if err != nil {
		return err
	}
	var podSpec corev1.PodSpec
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstrPodSpec, &podSpec)
	if err != nil {
		return err
	}
	dependencies = resourceutil.GetPodsDependencies(podSpec)

	// NOTE: ConfigMaps and Secrets that are used as depedency for the resource but do not exist
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
	case "Deployment":
		path = []string{"spec", "template", "metadata", "annotations"}
	case "CronJob":
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

func ensureDeployAll(res *resourceutil.Resource, currentTime time.Time) error {
	var path []string
	switch res.GroupVersionKind.Kind {
	case "Deployment":
		path = []string{"spec", "template", "metadata", "annotations"}
	case "CronJob":
		path = []string{"spec", "jobTemplate", "spec", "template", "metadata", "annotations"}
	}
	currentAnnotations, found, err := unstructured.NestedStringMap(res.Object.Object, path...)
	if err != nil {
		return err
	}
	if !found {
		currentAnnotations = make(map[string]string)
	}
	currentAnnotations[resourceutil.GetMiaAnnotation(deployChecksum)] = resourceutil.GetChecksum([]byte(currentTime.Format(time.RFC3339)))
	return unstructured.SetNestedStringMap(res.Object.Object, currentAnnotations, path...)
}

// cleanup removes the resources no longer deployed by `mlp` and updates
// the secret in the cluster with the updated set of resources
func cleanup(clients *k8sClients, namespace string, resources []resourceutil.Resource) error {
	actual := makeResourceMap(resources)

	old, err := getOldResourceMap(clients, namespace)
	if err != nil {
		return err
	}

	// Prune only if it is not the first release
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
			} else {
				return err
			}
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
				return errors.New(fmt.Sprintf("no namespace passed and no namespace in resource: %s %s", res.GroupVersionKind.Kind, res.Object.GetName()))
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
		err := decoratedApply(clients, res, deployConfig)
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

func createJobFromCronjob(k8sClient dynamic.Interface, res *unstructured.Unstructured) (string, error) {

	var cronjobObj batchv1beta1.CronJob
	err := runtime.DefaultUnstructuredConverter.
		FromUnstructured(res.Object, &cronjobObj)
	if err != nil {
		return "", fmt.Errorf("error in conversion to Cronjob")
	}
	annotations := make(map[string]string)
	annotations["cronjob.kubernetes.io/instantiate"] = "manual"
	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{APIVersion: batchv1.SchemeGroupVersion.String(), Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{
			// Use this instead of Name field to avoid name conflicts
			GenerateName: res.GetName() + "-",
			Annotations:  annotations,
			Labels:       cronjobObj.Spec.JobTemplate.Labels,

			// TODO: decide if it necessary to include it or not. At the moment it
			// prevents the pod creation saying that it cannot mount the default token
			// inside the container
			//
			// OwnerReferences: []metav1.OwnerReference{
			// 	{
			// 		APIVersion: batchv1beta1.SchemeGroupVersion.String(),
			// 		Kind:       cronjobObj.Kind,
			// 		Name:       cronjobObj.GetName(),
			// 		UID:        cronjobObj.GetUID(),
			// 	},
			// },
		},
		Spec: cronjobObj.Spec.JobTemplate.Spec,
	}

	fmt.Printf("Creating job from cronjob: %s\n", res.GetName())

	unstrCurrentObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&job)
	if err != nil {
		return "", err
	}

	jobCreated, err := k8sClient.Resource(gvrJobs).
		Namespace(res.GetNamespace()).
		Create(context.Background(),
			&unstructured.Unstructured{
				Object: unstrCurrentObj,
			},
			metav1.CreateOptions{})
	if err != nil {
		return "", err
	}

	return jobCreated.GetName(), nil
}

// createPatch returns the patch to be used in order to update the resource inside the cluster.
// The function performs a Three Way Merge Patch with the last applied configuration written in the
// object annotation, the actual resource state deployed inside the cluster and the desired state after
// the update.
func createPatch(currentObj unstructured.Unstructured, target resourceutil.Resource) ([]byte, types.PatchType, error) {
	// Get the config in the annotation
	original := currentObj.GetAnnotations()[corev1.LastAppliedConfigAnnotation]

	// Get the desired configuration
	obj := target.Object.DeepCopy()
	objAnn := obj.GetAnnotations()
	_, found := objAnn[corev1.LastAppliedConfigAnnotation]
	if found {
		delete(objAnn, corev1.LastAppliedConfigAnnotation)
		obj.SetAnnotations(objAnn)
	} else {
		objAnn = make(map[string]string)
	}
	objEncoded, err := obj.MarshalJSON()
	if err != nil {
		return nil, types.StrategicMergePatchType, err
	}
	objAnn[corev1.LastAppliedConfigAnnotation] = string(objEncoded)
	obj.SetAnnotations(objAnn)
	desired, err := obj.MarshalJSON()
	if err != nil {
		return nil, types.StrategicMergePatchType, err
	}
	// Get the resource in the cluster
	current, err := currentObj.MarshalJSON()
	if err != nil {
		return nil, types.StrategicMergePatchType, errors.Wrap(err, "serializing live configuration")
	}

	// Get the resource scheme
	versionedObject, err := scheme.Scheme.New(*target.GroupVersionKind)

	// use a three way json merge if the resource is a CRD
	if runtime.IsNotRegisteredError(err) {
		// fall back to generic JSON merge patch
		patchType := types.MergePatchType
		preconditions := []mergepatch.PreconditionFunc{mergepatch.RequireKeyUnchanged("apiVersion"),
			mergepatch.RequireKeyUnchanged("kind"), mergepatch.RequireMetadataKeyUnchanged("name")}
		patch, err := jsonmergepatch.CreateThreeWayJSONMergePatch([]byte(original), desired, current, preconditions...)
		return patch, patchType, err
	} else if err != nil {
		return nil, types.StrategicMergePatchType, err
	}

	patchMeta, err := strategicpatch.NewPatchMetaFromStruct(versionedObject)
	if err != nil {
		return nil, types.StrategicMergePatchType, errors.Wrap(err, "unable to create patch metadata from object")
	}

	patch, err := strategicpatch.CreateThreeWayMergePatch([]byte(original), desired, current, patchMeta, true)
	return patch, types.StrategicMergePatchType, err
}

// EnsureSmartDeploy merge, if present, the "mia-platform.eu/deploy-checksum" annotation from the Kubernetes cluster
// into the target Resource that is going to be released.
func ensureSmartDeploy(onClusterResource *unstructured.Unstructured, target *resourceutil.Resource) error {
	var path []string
	switch target.GroupVersionKind.Kind {
	case "Deployment":
		path = []string{"spec", "template", "metadata", "annotations"}
	case "CronJob":
		path = []string{"spec", "jobTemplate", "spec", "template", "metadata", "annotations"}
	}

	currentAnn, found, err := unstructured.NestedStringMap(onClusterResource.Object, path...)
	if err != nil {
		return err
	}
	if !found {
		currentAnn = make(map[string]string)
	}

	// If deployChecksum annotation is not found early returns avoiding creating an
	// empty annotation causing a pod restart
	depChecksum, found := currentAnn[resourceutil.GetMiaAnnotation(deployChecksum)]
	if !found {
		return nil
	}

	targetAnn, found, err := unstructured.NestedStringMap(target.Object.Object, path...)
	if err != nil {
		return err
	}
	if !found {
		targetAnn = make(map[string]string)
	}
	targetAnn[resourceutil.GetMiaAnnotation(deployChecksum)] = depChecksum
	err = unstructured.SetNestedStringMap(target.Object.Object, targetAnn, path...)
	if err != nil {
		return err
	}

	return nil
}

func checkIfCreateJob(k8sClient dynamic.Interface, currentObj *unstructured.Unstructured, target resourceutil.Resource) error {
	original := currentObj.GetAnnotations()[corev1.LastAppliedConfigAnnotation]

	obj := target.Object.DeepCopy()
	objAnn := obj.GetAnnotations()
	_, found := objAnn[corev1.LastAppliedConfigAnnotation]
	if found {
		delete(objAnn, corev1.LastAppliedConfigAnnotation)
		obj.SetAnnotations(objAnn)
	}
	desired, err := obj.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "serializing target configuration")
	}

	if !bytes.Equal([]byte(original), desired) {
		if err := cronJobAutoCreate(k8sClient, &target.Object); err != nil {
			return errors.Wrap(err, "failed on cronJobAutoCreate")
		}
	}
	return nil
}

// Create a Job from every CronJob having the mia-platform.eu/autocreate annotation set to true
func cronJobAutoCreate(k8sClient dynamic.Interface, res *unstructured.Unstructured) error {
	if res.GetKind() != "CronJob" {
		return nil
	}
	val, ok := res.GetAnnotations()[resourceutil.GetMiaAnnotation("autocreate")]
	if !ok || val != "true" {
		return nil
	}

	if _, err := createJobFromCronjob(k8sClient, res); err != nil {
		return err
	}
	return nil
}
