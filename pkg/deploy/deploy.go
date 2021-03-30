package deploy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

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
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util"
	"sigs.k8s.io/yaml"
)

const (
	dependenciesChecksum = "dependencies-checksum"
	deployChecksum = "deploy-checksum"
	smartDeploy    = "smart_deploy"
	deployAll      = "deploy_all"
)

// Run execute the deploy command from cli
func Run(inputPaths []string, deployConfig utils.DeployConfig, opts *utils.Options) {
	currentTime := time.Now()
	filePaths, err := utils.ExtractYAMLFiles(inputPaths)
	utils.CheckError(err)

	resources, err := resourceutil.MakeResources(opts, filePaths)
	utils.CheckError(err)
	err = prepareResources(deployConfig.DeployType, resources, currentTime)
	utils.CheckError(err)

	err = deploy(opts.Config, opts.Namespace, resources, deployConfig)
	utils.CheckError(err)

	err = cleanup(opts, resources)
	utils.CheckError(err)
}

func prepareResources(deployType string, resources []resourceutil.Resource, currentTime time.Time) error {
	configMapMap, secretMap, err := resourceutil.MapSecretAndConfigMap(resources)
	utils.CheckError(err)

	for _, res := range resources {
		if res.Head.Kind != "Deployment" && res.Head.Kind != "CronJob" {
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
	var annotations map[string]string
	var dependencies = map[string][]string{}

	switch res.Head.Kind {
	case "Deployment":
		deployment, err := resourceutil.GetAppsv1DeploymentFromObject(res.Info.Object)
		if err != nil {
			fmt.Printf("resource %s: %s", res.Name, err.Error())
		}
		dependencies = resourceutil.GetPodsDependencies(deployment.Spec.Template.Spec)
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		res.Info.Object = deployment
		annotations = deployment.Spec.Template.Annotations
	case "CronJob":
		cronJob, err := resourceutil.GetBatchapiv1beta1CronJobFromObject(res.Info.Object)
		if err != nil {
			fmt.Printf("resource %s: %s", res.Name, err.Error())
		}
		dependencies = resourceutil.GetPodsDependencies(cronJob.Spec.JobTemplate.Spec.Template.Spec)
		if cronJob.Spec.JobTemplate.Spec.Template.Annotations == nil {
			cronJob.Spec.JobTemplate.Spec.Template.Annotations = make(map[string]string)
		}
		res.Info.Object = cronJob
		annotations = cronJob.Spec.JobTemplate.Spec.Template.Annotations
	}

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

	annotations[resourceutil.GetMiaAnnotation(dependenciesChecksum)] = fmt.Sprintf("%s", jsonCheckSumMap)
	return nil
}

func ensureDeployAll(res *resourceutil.Resource, currentTime time.Time) error {
	var annotations map[string]string

	// Handle only Deployment and CronJob because:
	// "Secret" and "ConfigMap" is unnecessary and "Job" is immutable after created.
	switch res.Head.Kind {
	case "Deployment":
		deployment, err := resourceutil.GetAppsv1DeploymentFromObject(res.Info.Object)
		if err != nil {
			return fmt.Errorf("resource %s: %s", res.Name, err.Error())
		}
		if deployment.Spec.Template.Annotations == nil {
			deployment.Spec.Template.Annotations = make(map[string]string)
		}
		res.Info.Object = deployment
		annotations = deployment.Spec.Template.Annotations
	case "CronJob":
		cronJob, err := resourceutil.GetBatchapiv1beta1CronJobFromObject(res.Info.Object)
		if err != nil {
			return fmt.Errorf("resource %s: %s", res.Name, err.Error())
		}
		if cronJob.Spec.JobTemplate.Spec.Template.Annotations == nil {
			cronJob.Spec.JobTemplate.Spec.Template.Annotations = make(map[string]string)
		}
		res.Info.Object = cronJob
		annotations = cronJob.Spec.JobTemplate.Spec.Template.Annotations
	}
	annotations[resourceutil.GetMiaAnnotation(deployChecksum)] = resourceutil.GetChecksum([]byte(currentTime.Format(time.RFC3339)))
	return nil
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

		for _, resourceGroup := range deleteMap {
			builder := resourceutil.NewBuilder(opts.Config)
			err = prune(builder, opts.Namespace, resourceGroup)
			if err != nil {
				return err
			}
		}
	}
	_, err = updateResourceSecret(builder, opts.Namespace, actual)
	return err
}

func updateResourceSecret(infoGen resourceutil.InfoGenerator, namespace string, resources map[string]*ResourceList) (*apiv1.Secret, error) {
	secretContent, err := json.Marshal(resources)
	if err != nil {
		return nil, err
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
		return nil, err
	}

	secretInfo, err := infoGen.FromStream(bytes.NewBuffer(buf))

	if err != nil {
		return nil, err
	}

	helper := infoGen.NewHelper(secretInfo[0].Client, secretInfo[0].Mapping)

	if _, err = helper.Create(namespace, false, secretInfo[0].Object); err != nil {
		if apierrors.IsAlreadyExists(err) {
			_, err = helper.Replace(namespace, resourceSecretName, true, secretInfo[0].Object)

			if err != nil {
				return nil, err
			}
		}
	}
	return secret, nil
}

// prune resources no longer managed by `mlp`
func prune(infoGen resourceutil.InfoGenerator, namespace string, resourceGroup *ResourceList) error {

	infos, err := infoGen.FromNames(namespace, resourceGroup.Mapping.Resource, resourceGroup.Resources)

	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

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

		helper := infoGen.NewHelper(objectInfo.Client, objectInfo.Mapping)

		_, err = helper.Delete(objectInfo.Namespace, objectInfo.Name)

		if err != nil && !apierrors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

func deploy(config *genericclioptions.ConfigFlags, namespace string, resources []resourceutil.Resource, deployConfig utils.DeployConfig) error {

	builder := resourceutil.NewBuilder(config)
	// Check that the namespace exists
	_, err := ensureNamespaceExistance(builder, namespace)

	if err != nil {
		return err
	}

	// apply the resources
	for _, res := range resources {
		err := apply(builder, res, deployConfig)
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

func apply(infoGen resourceutil.InfoGenerator, res resourceutil.Resource, deployConfig utils.DeployConfig) error {

	var (
		currentObj runtime.Object
		err        error
	)

	helper := infoGen.NewHelper(res.Info.Client, res.Info.Mapping)

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

			if err = cronJobAutoCreate(infoGen, res); err != nil {
				return err
			}

			_, err = helper.Create(res.Info.Namespace, false, res.Info.Object)
		}
		return err
	}
	// Do not modify the resource if the annotation is set to `once`
	if res.Head.Metadata.Annotations[resourceutil.GetMiaAnnotation("deploy")] != "once" {

		// Replace only if it is a Secret or configmap otherwise patch the resource
		if res.Head.Kind == "Secret" || res.Head.Kind == "ConfigMap" {
			fmt.Printf("Replacing %s: %s\n", res.Head.Kind, res.Info.Name)
			_, err = helper.Replace(res.Info.Namespace, res.Info.Name, true, res.Info.Object)

		} else {

			fmt.Printf("Updating %s: %s\n", res.Head.Kind, res.Info.Name)

			if deployConfig.DeployType == smartDeploy {
				isNotUsingSemver, err := resourceutil.IsNotUsingSemver(&res)
				if err != nil {
					return errors.Wrap(err, "failed semver check")
				}

				if deployConfig.ForceDeployWhenNoSemver && isNotUsingSemver {
					if err := ensureDeployAll(&res, time.Now()); err != nil {
						return errors.Wrap(err, "failed ensure deploy all on resource not using semver")
					}
				} else {
					if err = ensureSmartDeploy(infoGen, currentObj, &res); err != nil {
						return errors.Wrap(err, "failed smart deploy ensure")
					}
				}
			}

			if res.Head.Kind == "CronJob" {
				if err := checkIfCreateJob(infoGen, currentObj, res); err != nil {
					return errors.Wrap(err, "failed check if create job")
				}
			}

			patch, patchType, err := createPatch(currentObj, res)

			// create the patch
			if err != nil {
				return errors.Wrap(err, "failed to create patch")
			}

			if _, err := helper.Patch(res.Info.Namespace, res.Info.Name, patchType, patch, nil); err != nil {
				return errors.Wrap(err, "failed to patch")
			}
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

// EnsureSmartDeploy merge, if present, the "mia-platform.eu/deploy-checksum" annotation from the Kubernetes cluster
// into the target Resource that is going to be released.
func ensureSmartDeploy(infoGen resourceutil.InfoGenerator, currentObj runtime.Object, target *resourceutil.Resource) error {
	switch target.Head.Kind {
	case "Deployment":
		currentDeployment, err := resourceutil.GetAppsv1DeploymentFromObject(currentObj)
		if err != nil {
			return fmt.Errorf("current resource %s: %s", target.Name, err.Error())
		}

		desiredDeployment, err := resourceutil.GetAppsv1DeploymentFromObject(target.Info.Object)
		if err != nil {
			return fmt.Errorf("target resource %s: %s", target.Name, err.Error())
		}

		currentAnnotation := currentDeployment.Spec.Template.Annotations
		if currentAnnotation != nil && currentAnnotation[resourceutil.GetMiaAnnotation(deployChecksum)] != "" {
			if desiredDeployment.Spec.Template.Annotations == nil {
				desiredDeployment.Spec.Template.Annotations = make(map[string]string)
			}

			desiredDeployment.Spec.Template.Annotations[resourceutil.GetMiaAnnotation(deployChecksum)] = currentAnnotation[resourceutil.GetMiaAnnotation(deployChecksum)]
			target.Info.Object = desiredDeployment
		}
	case "CronJob":
		currentCronJob, err := resourceutil.GetBatchapiv1beta1CronJobFromObject(currentObj)
		if err != nil {
			return fmt.Errorf("current resource %s: %s", target.Name, err.Error())
		}

		desiredCronJob, err := resourceutil.GetBatchapiv1beta1CronJobFromObject(target.Info.Object)
		if err != nil {
			return fmt.Errorf("target resource %s: %s", target.Name, err.Error())
		}

		currentAnnotation := currentCronJob.Spec.JobTemplate.Spec.Template.Annotations
		if currentAnnotation != nil && currentAnnotation[resourceutil.GetMiaAnnotation(deployChecksum)] != "" {
			if desiredCronJob.Spec.JobTemplate.Spec.Template.Annotations == nil {
				desiredCronJob.Spec.JobTemplate.Spec.Template.Annotations = make(map[string]string)
			}
			desiredCronJob.Spec.JobTemplate.Spec.Template.Annotations[resourceutil.GetMiaAnnotation(deployChecksum)] = currentAnnotation[resourceutil.GetMiaAnnotation(deployChecksum)]
			target.Info.Object = desiredCronJob
		}
	}

	return nil
}

func checkIfCreateJob(infoGen resourceutil.InfoGenerator, currentObj runtime.Object, target resourceutil.Resource) error {
	// Get the config in the annotation
	original, err := util.GetOriginalConfiguration(currentObj)
	if err != nil {
		return errors.Wrap(err, "serializing original configuration")
	}

	// Get the desired configuration
	desired, err := util.GetModifiedConfiguration(target.Info.Object, false, unstructured.UnstructuredJSONScheme)
	if err != nil {
		return errors.Wrap(err, "serializing target configuration")
	}

	if !bytes.Equal(original, desired) {
		if err := cronJobAutoCreate(infoGen, target); err != nil {
			return errors.Wrap(err, "failed on cronJobAutoCreate")
		}
	}
	return nil
}

// Create a Job from every CronJob having the mia-platform.eu/autocreate annotation set to true
func cronJobAutoCreate(infoGen resourceutil.InfoGenerator, res resourceutil.Resource) error {
	if res.Head.Kind != "CronJob" {
		return nil
	}
	val, ok := res.Head.Metadata.Annotations[resourceutil.GetMiaAnnotation("autocreate")]
	if !ok || val != "true" {
		return nil
	}

	if _, err := createJobFromCronjob(infoGen, res); err != nil {
		return err
	}
	return nil
}
