// Copyright 2022 Mia srl
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
	"fmt"
	"time"

	externalsecretsv1beta1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1beta1"
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
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubectl/pkg/scheme"
)

type applyFunction func(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error

const (
	deployChecksum = "deploy-checksum"
	smartDeploy    = "smart_deploy"
)

var (
	deleteBeforeApplyAnnotation = resourceutil.GetMiaAnnotation("delete-before-apply")
	awaitCompletionAnnotation   = resourceutil.GetMiaAnnotation("await-completion")
	apply                       = decorateApplyResource(withAwaitableResource, withDeletableResource)
)

func decorateApplyResource(decorators ...func(applyFunction) applyFunction) applyFunction {
	decoratedApplyFn := applyResource
	for _, f := range decorators {
		decoratedApplyFn = f(decoratedApplyFn)
	}
	return decoratedApplyFn
}

// withDeletableResource is an apply function decorator that deletes resources
// annotated with deleteBeforeApplyAnnotation before applying them to the cluster
func withDeletableResource(apply applyFunction) applyFunction {
	return func(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error {
		_, deleteBeforeApplyFound := res.Object.GetAnnotations()[deleteBeforeApplyAnnotation]
		if deleteBeforeApplyFound {
			gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, res.Object.GroupVersionKind())
			if err != nil {
				return err
			}

			fmt.Printf("Deleting resource %s before apply\n", res.Object.GetName())

			err = clients.dynamic.Resource(gvr).
				Delete(context.TODO(), res.Object.GetName(), metav1.DeleteOptions{})
			if err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		}

		return apply(clients, res, deployConfig)
	}
}

// withAwaitableResource is an apply function decorator that awaits resources
// decorated with awaitCompletionAnnotation for completion after they are
// applied on the cluster
func withAwaitableResource(apply applyFunction) applyFunction {
	return func(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error {
		gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, res.Object.GroupVersionKind())
		if err != nil {
			return err
		}

		// register a watcher and starts to listen for events for the gvr
		// if res is annotated with awaitCompletionAnnotation
		var watchEvents <-chan watch.Event
		startTime := time.Now().Round(time.Second)
		awaitCompletionValue, awaitCompletionFound := res.Object.GetAnnotations()[awaitCompletionAnnotation]
		if awaitCompletionFound {
			watcher, err := clients.dynamic.Resource(gvr).
				Namespace(res.Object.GetNamespace()).
				Watch(context.TODO(), metav1.ListOptions{})
			if err != nil {
				return err
			}
			watchEvents = watcher.ResultChan()
			fmt.Printf("Registered a watcher for resource: %s.%s.%s having name %s (time: %s)\n", gvr.Group, gvr.Version, gvr.Resource, res.Object.GetName(), startTime)
			defer watcher.Stop()
		}

		// actually apply the resource
		if err := apply(clients, res, deployConfig); err != nil {
			return err
		}

		// return if no event channel has been set
		if watchEvents == nil {
			return nil
		}

		// parse timeout from annotation value
		timeout, err := time.ParseDuration(awaitCompletionValue)
		if err != nil {
			msg := fmt.Sprintf("Error in %s annotation value for resource \"%s\": must be a valid duration", awaitCompletionAnnotation, res.Object.GetName())
			return errors.Wrap(err, msg)
		}

		if err := assertAwaitSupportedForThisResource(res); err != nil {
			return err
		}

		// consume watcher events and wait for the resource to complete or exit because of timeout
		for {
			select {
			case event, ok := <-watchEvents:
				if !ok {
					return errors.New("Watch channel closed unexpectedly")
				}
				isCompleted, err := handleResourceCompletionEvent(res, &event, startTime)
				if err != nil {
					msg := "Error while watching resource events"
					return errors.Wrap(err, msg)
				}

				if isCompleted {
					return nil
				}
			case <-time.NewTimer(timeout).C:
				fmt.Printf("Timeout received while waiting for resource %s completion\n", res.Object.GetName())
				return nil
			}
		}
	}
}

func assertAwaitSupportedForThisResource(res resourceutil.Resource) error {
	_, err := handleResourceCompletionEvent(res, nil, time.Now())
	return err
}

// handleResourceCompletionEvent takes the target resource, the watch event and
// the initial watch time as arguments. It returns (true, nil) when the given
// resource has completed in the given event. If the event is nil returns (false, nil)
// when the resource supports events watching otherwise returns (false, error).
func handleResourceCompletionEvent(res resourceutil.Resource, event *watch.Event, startTime time.Time) (bool, error) {
	switch res.GroupVersionKind.Kind {
	case "Job":
		// only manage watch.Modified events
		if event == nil || event.Type != watch.Modified {
			return false, nil
		}
		// convert resources into jobs
		var jobFromRes, jobFromEvent batchv1.Job

		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object.Object, &jobFromRes); err != nil {
			return false, err
		}

		u, ok := event.Object.(*unstructured.Unstructured)
		if !ok {
			msg := "Cannot convert object event to unstructured while handling events for Job"
			return false, errors.New(msg)
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &jobFromEvent); err != nil {
			return false, err
		}
		// if job from event is the one we are listening for changes
		if jobFromEvent.Name != jobFromRes.Name {
			return false, nil
		}
		// check if job has completed after start time
		if completedAt := jobFromEvent.Status.CompletionTime; completedAt != nil && !completedAt.Time.Before(startTime) {
			fmt.Println("Job completed:", jobFromEvent.Name)
			return true, nil
		}

		return false, nil
	case "ExternalSecret":
		if event == nil || event.Type != watch.Modified {
			return false, nil
		}

		var extsecFromRes, extsecFromEvent externalsecretsv1beta1.ExternalSecret
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.Object.Object, &extsecFromRes); err != nil {
			return false, err
		}

		u, ok := event.Object.(*unstructured.Unstructured)
		if !ok {
			msg := "Cannot convert object from event into unstructured object while handling ExternalSecret events"
			return false, errors.New(msg)
		}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, &extsecFromEvent); err != nil {
			return false, err
		}

		if extsecFromEvent.Name != extsecFromRes.Name {
			return false, nil
		}

		if refreshedAt := extsecFromEvent.Status.RefreshTime; !refreshedAt.Time.Before(startTime) {
			fmt.Println("ExternalSecret completed:", extsecFromEvent.Name)
			return true, nil
		} else {
			fmt.Printf("Found old refresh event: %s\n", refreshedAt)
		}

		return false, nil
	default:
		msg := fmt.Sprintf("No watch handler for resource %s.%s.%s having name %s", res.GroupVersionKind.Group, res.GroupVersionKind.Version, res.GroupVersionKind.Kind, res.Object.GetName())
		return false, errors.New(msg)
	}
}

func applyResource(clients *k8sClients, res resourceutil.Resource, deployConfig utils.DeployConfig) error {

	gvr, err := resourceutil.FromGVKtoGVR(clients.discovery, res.Object.GroupVersionKind())
	if err != nil {
		return err
	}

	var onClusterObj *unstructured.Unstructured
	if onClusterObj, err = clients.dynamic.Resource(gvr).
		Namespace(res.Object.GetNamespace()).
		Get(context.Background(), res.Object.GetName(), metav1.GetOptions{}); err != nil {
		// create the resource only if it is not present in the cluster
		if apierrors.IsNotFound(err) {
			fmt.Printf("Creating %s: %s\n", res.Object.GetKind(), res.Object.GetName())

			// creates kubectl.kubernetes.io/last-applied-configuration annotation
			// inside the resource except for Secrets and ConfigMaps
			if res.Object.GetKind() != "Secret" && res.Object.GetKind() != "ConfigMap" {
				orignAnn := res.Object.GetAnnotations()
				if orignAnn == nil {
					orignAnn = make(map[string]string)
				}
				objJson, err := res.Object.MarshalJSON()
				if err != nil {
					return err
				}
				orignAnn[corev1.LastAppliedConfigAnnotation] = string(objJson)
				res.Object.SetAnnotations(orignAnn)
			}

			if err = cronJobAutoCreate(clients.dynamic, &res.Object); err != nil {
				return err
			}

			_, err = clients.dynamic.Resource(gvr).
				Namespace(res.Object.GetNamespace()).
				Create(context.Background(),
					&res.Object,
					metav1.CreateOptions{})
		}
		return err
	}

	// Do not modify the resource if is already present on cluster and the annotation is set to "once"
	if res.Object.GetAnnotations()[resourceutil.GetMiaAnnotation("deploy")] != "once" {

		// Replace only if it is a Secret or configmap otherwise patch the resource
		if res.Object.GetKind() == "Secret" || res.Object.GetKind() == "ConfigMap" {
			fmt.Printf("Replacing %s: %s\n", res.Object.GetKind(), res.Object.GetName())

			_, err = clients.dynamic.Resource(gvr).
				Namespace(res.Object.GetNamespace()).
				Update(context.Background(),
					&res.Object,
					metav1.UpdateOptions{})

		} else {

			fmt.Printf("Updating %s: %s\n", res.Object.GetKind(), res.Object.GetName())

			if deployConfig.DeployType == smartDeploy && (res.Object.GetKind() == "CronJob" || res.Object.GetKind() == "Deployment") {
				isNotUsingSemver, err := resourceutil.IsNotUsingSemver(&res)
				if err != nil {
					return errors.Wrap(err, "failed semver check")
				}

				if deployConfig.ForceDeployWhenNoSemver && isNotUsingSemver {
					if err := ensureDeployAll(&res, time.Now()); err != nil {
						return errors.Wrap(err, "failed ensure deploy all on resource not using semver")
					}
				} else {
					if err = ensureSmartDeploy(onClusterObj, &res); err != nil {
						return errors.Wrap(err, "failed smart deploy ensure")
					}
				}
			}

			if res.Object.GetKind() == "CronJob" {
				if err := checkIfCreateJob(clients.dynamic, onClusterObj, res); err != nil {
					return errors.Wrap(err, "failed check if create job")
				}
			}

			patch, patchType, err := createPatch(*onClusterObj, res)

			// create the patch
			if err != nil {
				return errors.Wrap(err, "failed to create patch")
			}

			if _, err := clients.dynamic.Resource(gvr).
				Namespace(res.Object.GetNamespace()).
				Patch(context.Background(),
					res.Object.GetName(), patchType, patch, metav1.PatchOptions{}); err != nil {
				return errors.Wrap(err, "failed to patch")
			}
		}
		return err
	}
	return nil
}

// annotateWithLastApplied annotates a given resource with corev1.LastAppliedConfigAnnotation
func annotateWithLastApplied(res resourceutil.Resource) (unstructured.Unstructured, error) {
	annotatedRes := res.Object.DeepCopy()
	annotations := annotatedRes.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	if _, found := annotations[corev1.LastAppliedConfigAnnotation]; found {
		delete(annotations, corev1.LastAppliedConfigAnnotation)
		annotatedRes.SetAnnotations(annotations)
	}

	resJson, err := annotatedRes.MarshalJSON()
	if err != nil {
		return unstructured.Unstructured{}, err
	}

	annotations[corev1.LastAppliedConfigAnnotation] = string(resJson)
	annotatedRes.SetAnnotations(annotations)

	return *annotatedRes, nil
}

// createPatch returns the patch to be used in order to update the resource inside the cluster.
// The function performs a Three Way Merge Patch with the last applied configuration written in the
// object annotation, the actual resource state deployed inside the cluster and the desired state after
// the update.
func createPatch(currentObj unstructured.Unstructured, target resourceutil.Resource) ([]byte, types.PatchType, error) {
	// Get last applied config from current object annotation
	lastAppliedConfigJson := currentObj.GetAnnotations()[corev1.LastAppliedConfigAnnotation]

	// Get the desired configuration
	annotatedTarget, err := annotateWithLastApplied(target)
	if err != nil {
		return nil, types.StrategicMergePatchType, err
	}
	targetJson, err := annotatedTarget.MarshalJSON()
	if err != nil {
		return nil, types.StrategicMergePatchType, err
	}

	// Get the resource in the cluster
	currentJson, err := currentObj.MarshalJSON()
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
		patch, err := jsonmergepatch.CreateThreeWayJSONMergePatch([]byte(lastAppliedConfigJson), targetJson, currentJson, preconditions...)
		return patch, patchType, err
	} else if err != nil {
		return nil, types.StrategicMergePatchType, err
	}

	patchMeta, err := strategicpatch.NewPatchMetaFromStruct(versionedObject)
	if err != nil {
		return nil, types.StrategicMergePatchType, errors.Wrap(err, "unable to create patch metadata from object")
	}

	patch, err := strategicpatch.CreateThreeWayMergePatch([]byte(lastAppliedConfigJson), targetJson, currentJson, patchMeta, true)
	return patch, types.StrategicMergePatchType, err
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
