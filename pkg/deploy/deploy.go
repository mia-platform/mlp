package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/pkg/errors"
	batchapiv1 "k8s.io/api/batch/v1"
	batchapiv1beta1 "k8s.io/api/batch/v1beta1"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util"
	"sigs.k8s.io/yaml"
)

type resHelper interface {
	Get(namespace, name string) (runtime.Object, error)
	Create(namespace string, modify bool, obj runtime.Object) (runtime.Object, error)
	Replace(namespace, name string, overwrite bool, obj runtime.Object) (runtime.Object, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, options *metav1.PatchOptions) (runtime.Object, error)
}

// Run execute the deploy command from cli
func Run(inputPaths []string, opts *utils.Options) {

	filePaths, err := utils.ExtractYAMLFiles(inputPaths)
	utils.CheckError(err)

	resources, err := resourceutil.MakeResources(opts, filePaths)
	utils.CheckError(err)

	err = deploy(opts.Config, opts.Namespace, resources)
	utils.CheckError(err)

	err = cleanup(opts, resources)
	utils.CheckError(err)
}

// cleanup removes the resources no longer deployed by `mlp` and updates
// the secret in the cluster with the updated set of resources
func cleanup(opts *utils.Options, resources []resourceutil.Resource) error {
	actual := makeResourceMap(resources)

	builder := resourceutil.NewBuilder(opts.Config)

	old, err := getOldResourceMap(builder, opts.Namespace)
	if err != nil {
		return err
	}

	// Prune only if it is not the first release
	if len(old) != 0 {
		deleteMap := deletedResources(actual, old)

		err = prune(opts.Config, opts.Namespace, deleteMap)
		if err != nil {
			return err
		}
	}

	return updateResourceSecret(builder, opts.Namespace, actual)
}

func updateResourceSecret(infoGen resourceutil.InfoGenerator, namespace string, resources map[string]*ResourceList) error {
	secretContent, err := json.Marshal(resources)
	if err != nil {
		return err
	}
	secret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceSecretName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		Data:     map[string][]byte{"resources": secretContent},
	}

	buf, err := yaml.Marshal(secret)

	if err != nil {
		return err
	}

	secretInfo, err := infoGen.FromStream(bytes.NewBuffer(buf))

	if err != nil {
		return err
	}

	helper := infoGen.NewHelper(secretInfo[0].Client, secretInfo[0].Mapping)

	if _, err = helper.Create(namespace, false, secretInfo[0].Object); err != nil {
		if apierrors.IsAlreadyExists(err) {
			_, err = helper.Replace(namespace, resourceSecretName, true, secretInfo[0].Object)

			if err != nil {
				return err
			}
		}
	}
	return nil
}

// prune resources no longer managed by `mlp`
func prune(config *genericclioptions.ConfigFlags, namespace string, deleteMap map[string]*ResourceList) error {
	for _, resourceGroup := range deleteMap {
		builder := resourceutil.NewBuilder(config)

		infos, err := builder.FromNames(namespace, resourceGroup.Mapping.Resource, resourceGroup.Resources)
		utils.CheckError(err)

		for _, objectInfo := range infos {
			fmt.Printf("deleting: %v %v\n", resourceGroup.Kind, objectInfo.Name)

			objMeta, err := meta.Accessor(objectInfo.Object)
			if err != nil {
				return err
			}

			// delete the object only if the resource has the managed by MIA label
			if objMeta.GetLabels()[resourceutil.ManagedByLabel] != resourceutil.ManagedByMia {
				continue
			}

			helper := resource.NewHelper(objectInfo.Client, objectInfo.Mapping)

			_, err = helper.Delete(objectInfo.Namespace, objectInfo.Name)

			if err != nil {
				return err
			}
		}
	}

	return nil
}

func deploy(config *genericclioptions.ConfigFlags, namespace string, resources []resourceutil.Resource) error {

	builder := resourceutil.NewBuilder(config)
	// Check that the namespace exists
	_, err := ensureNamespaceExistance(builder, namespace)

	if err != nil {
		return err
	}

	// apply the resources
	for _, res := range resources {
		err := apply(builder, res)
		if err != nil {
			return err
		}
	}
	return nil
}

func ensureNamespaceExistance(infoGen resourceutil.InfoGenerator, namespace string) (created *apiv1.Namespace, err error) {

	ns := &apiv1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	buf, err := yaml.Marshal(ns)

	if err != nil {
		return nil, err
	}
	namespaceInfo, err := infoGen.FromStream(bytes.NewBuffer(buf))

	if err != nil {
		return nil, err
	}

	helper := infoGen.NewHelper(namespaceInfo[0].Client, namespaceInfo[0].Mapping)

	if _, err := helper.Create(namespace, false, namespaceInfo[0].Object); err != nil && !apierrors.IsAlreadyExists(err) {
		return nil, err
	}

	return ns, err
}

func createJobFromCronjob(infoGen resourceutil.InfoGenerator, res resourceutil.Resource) (*batchapiv1.Job, error) {
	cronJobMetadata, err := meta.Accessor(res.Info.Object)
	if err != nil {
		return nil, err
	}

	uncastVersionedObj, err := scheme.Scheme.ConvertToVersion(res.Info.Object, batchapiv1beta1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	cronjobObj, ok := uncastVersionedObj.(*batchapiv1beta1.CronJob)
	if !ok {
		return nil, fmt.Errorf("Error in conversion to Cronjob")
	}

	// TODO useless if OwnerReferences are not used
	// cronUUID := uuid.NewUUID()

	// // use the old UID if the cron already exists
	// if oldCron, err := client.BatchV1beta1().CronJobs(options.Namespace).Get(context.TODO(), cronJobMetadata.GetName(), metav1.GetOptions{}); err == nil {
	// 	cronUUID = oldCron.GetUID()
	// }

	// if err != nil && !apierrors.IsNotFound(err) {
	// 	return nil, err
	// }

	// cronjobObj.SetUID(cronUUID)

	annotations := make(map[string]string)
	annotations["cronjob.kubernetes.io/instantiate"] = "manual"
	job := &batchapiv1.Job{
		TypeMeta: metav1.TypeMeta{APIVersion: batchapiv1.SchemeGroupVersion.String(), Kind: "Job"},
		ObjectMeta: metav1.ObjectMeta{
			// Use this instead of Name field to avoid name conflicts
			GenerateName: cronJobMetadata.GetName() + "-",
			Annotations:  annotations,
			Labels:       cronjobObj.Spec.JobTemplate.Labels,

			// TODO: decide if it necessary to include it or not. At the moment it
			// prevents the pod creation saying that it cannot mount the default token
			// inside the container
			//
			// OwnerReferences: []metav1.OwnerReference{
			// 	{
			// 		APIVersion: batchapiv1beta1.SchemeGroupVersion.String(),
			// 		Kind:       cronjobObj.Kind,
			// 		Name:       cronjobObj.GetName(),
			// 		UID:        cronjobObj.GetUID(),
			// 	},
			// },
		},
		Spec: cronjobObj.Spec.JobTemplate.Spec,
	}

	buf, err := yaml.Marshal(job)

	if err != nil {
		return nil, err
	}

	jobInfo, err := infoGen.FromStream(bytes.NewBuffer(buf))

	if err != nil {
		return nil, err
	}

	helper := infoGen.NewHelper(jobInfo[0].Client, jobInfo[0].Mapping)

	fmt.Printf("Creating job from cronjob: %s\n", res.Name)

	if _, err := helper.Create(res.Namespace, false, jobInfo[0].Object); err != nil {
		return nil, err
	}

	return job, nil
}

func apply(infoGen resourceutil.InfoGenerator, res resourceutil.Resource) error {

	var (
		currentObj runtime.Object
		err        error
	)

	helper := infoGen.NewHelper(res.Info.Client, res.Info.Mapping)

	// Create a Job from every CronJob having the mia-platform.eu/autocreate annotation set to true
	if res.Head.Kind == "CronJob" {
		if val, ok := res.Head.Metadata.Annotations["mia-platform.eu/autocreate"]; ok && val == "true" {
			_, err := createJobFromCronjob(infoGen, res)
			if err != nil {
				return err
			}
		}
	}

	if currentObj, err = helper.Get(res.Info.Namespace, res.Info.Name); err != nil {
		// create the resource only if it is not present in the cluster
		if apierrors.IsNotFound(err) {
			fmt.Printf("Creating %s: %s\n", res.Head.Kind, res.Name)

			// creates kubectl.kubernetes.io/last-applied-configuration annotation
			// inside the resource except for Secrets and ConfigMaps
			if res.Head.Kind != "Secret" && res.Head.Kind != "ConfigMap" {
				if err = util.CreateApplyAnnotation(res.Info.Object, unstructured.UnstructuredJSONScheme); err != nil {
					return err
				}
			}
			_, err = helper.Create(res.Info.Namespace, false, res.Info.Object)
		}
		return err
	}

	// Do not modify the resource if the annotation is set to `once`
	if res.Head.Metadata.Annotations["mia-platform.eu/deploy"] != "once" {

		// Replace only if it is a Secret or configmap otherwise patch the resource
		if res.Head.Kind == "Secret" || res.Head.Kind == "ConfigMap" {
			fmt.Printf("Replacing %s: %s\n", res.Head.Kind, res.Info.Name)
			_, err = helper.Replace(res.Info.Namespace, res.Info.Name, true, res.Info.Object)

		} else {

			fmt.Printf("Updating %s: %s\n", res.Head.Kind, res.Info.Name)

			patch, patchType, err := createPatch(currentObj, res)

			// create the patch

			if err != nil {
				return errors.Wrap(err, "failed to create patch")
			}

			_, err = helper.Patch(res.Info.Namespace, res.Info.Name, patchType, patch, nil)
		}
		return err
	}
	return nil
}

// createPatch returns the patch to be used in order to update the resource inside the cluster.
// The function performs a Three Way Merge Patch with the last applied configuration written in the
// object annotation, the actual resource state deployed inside the cluster and the desired state after
// the update.
func createPatch(currentObj runtime.Object, target resourceutil.Resource) ([]byte, types.PatchType, error) {

	// Get the config in the annotation
	original, err := util.GetOriginalConfiguration(currentObj)

	if err != nil {
		return nil, types.StrategicMergePatchType, errors.Wrap(err, "serializing original configuration")
	}

	// Get the desired configuration
	desired, err := util.GetModifiedConfiguration(target.Info.Object, true, unstructured.UnstructuredJSONScheme)
	if err != nil {
		return nil, types.StrategicMergePatchType, errors.Wrap(err, "serializing target configuration")
	}

	// Get the resource in the cluster
	current, err := json.Marshal(currentObj)
	if err != nil {
		return nil, types.StrategicMergePatchType, errors.Wrap(err, "serializing live configuration")
	}

	// Get the resource scheme
	versionedObject, err := scheme.Scheme.New(target.Info.Mapping.GroupVersionKind)

	// use a three way json merge if the resource is a CRD
	if runtime.IsNotRegisteredError(err) {
		// fall back to generic JSON merge patch
		patchType := types.MergePatchType
		preconditions := []mergepatch.PreconditionFunc{mergepatch.RequireKeyUnchanged("apiVersion"),
			mergepatch.RequireKeyUnchanged("kind"), mergepatch.RequireMetadataKeyUnchanged("name")}
		patch, err := jsonmergepatch.CreateThreeWayJSONMergePatch(original, desired, current, preconditions...)

		return patch, patchType, err
	}

	patchMeta, err := strategicpatch.NewPatchMetaFromStruct(versionedObject)
	if err != nil {
		return nil, types.StrategicMergePatchType, errors.Wrap(err, "unable to create patch metadata from object")
	}

	patch, err := strategicpatch.CreateThreeWayMergePatch(original, desired, current, patchMeta, true)
	return patch, types.StrategicMergePatchType, err
}
