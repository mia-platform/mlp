package deploy

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/client-go/kubernetes"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/pkg/errors"
	apiv1 "k8s.io/api/core/v1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/jsonmergepatch"
	"k8s.io/apimachinery/pkg/util/mergepatch"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/cli-runtime/pkg/resource"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/kubectl/pkg/util"
)

var options *utils.Options

type resHelper interface {
	Get(namespace, name string) (runtime.Object, error)
	Create(namespace string, modify bool, obj runtime.Object) (runtime.Object, error)
	Replace(namespace, name string, overwrite bool, obj runtime.Object) (runtime.Object, error)
	Patch(namespace, name string, pt types.PatchType, data []byte, options *metav1.PatchOptions) (runtime.Object, error)
}

// Run execute the deploy command from cli
func Run(inputPaths []string, opts *utils.Options) {
	options = opts
	filePaths, err := utils.ExtractYAMLFiles(inputPaths)
	utils.CheckError(err)

	resources, err := makeResources(filePaths)
	utils.CheckError(err)
	addInfos(&resources)
	err = deploy(resources)
	utils.CheckError(err)
}

func makeResources(filePaths []string) ([]resourceutil.Resource, error) {
	resources := []resourceutil.Resource{}
	for _, path := range filePaths {
		res, err := resourceutil.NewResource(path)
		if err != nil {
			return nil, err
		}
		resources = append(resources, *res)
	}

	resources = resourceutil.SortResourcesByKind(resources, nil)
	return resources, nil
}

func deploy(resources []resourceutil.Resource) error {

	// Check that the namespace exists
	config, err := options.Config.ToRESTConfig()
	utils.CheckError(err)

	client, err := kubernetes.NewForConfig(config)
	utils.CheckError(err)

	if _, err = client.CoreV1().Namespaces().Get(context.TODO(), options.Namespace, metav1.GetOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			fmt.Printf("Creating Namespace: %s\n", options.Namespace)

			ns := &apiv1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: options.Namespace},
				TypeMeta:   metav1.TypeMeta{Kind: "Namespace", APIVersion: "v1"},
			}

			_, err = client.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
			utils.CheckError(err)
		}
	}

	// apply the resources
	for _, res := range resources {
		helper := resource.NewHelper(res.Info.Client, res.Info.Mapping)
		err := apply(res, helper)
		if err != nil {
			return err
		}
	}
	return nil
}

func apply(res resourceutil.Resource, helper resHelper) error {

	var (
		currentObj runtime.Object
		err        error
	)

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

		// Replace only if it is a Secret or configmap otherwise path the resource
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

func addInfos(resources *[]resourceutil.Resource) {
	files := []string{}

	for _, resource := range *resources {
		files = append(files, resource.Filepath)
	}

	infos, err := resource.NewBuilder(options.Config).
		Unstructured().
		RequireObject(true).
		FilenameParam(false, &resource.FilenameOptions{Filenames: files, Recursive: false}).
		Flatten().
		Do().Infos()

	utils.CheckError(err)

	for i, v := range infos {
		v.Namespace = options.Namespace
		(*resources)[i].Info = v
	}
}
