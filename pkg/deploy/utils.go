package deploy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kubectl/pkg/scheme"
	"sigs.k8s.io/yaml"
)

const (
	resourceSecretName = "resources-deployed"
	resourceField      = "resources"
)

// ResourceList is the base block used to build the secret containing
// the resources deployed in the cluster.
type ResourceList struct {
	Kind      string `json:"kind"`
	Mapping   schema.GroupVersionResource
	Resources []string `json:"resources"`
}

// makeResourceMap groups the resources list by kind and embeds them in a `ResourceList` struct
func makeResourceMap(resources []resourceutil.Resource) map[string]*ResourceList {
	res := make(map[string]*ResourceList)

	for _, r := range resources {
		if _, ok := res[r.Head.Kind]; !ok {
			res[r.Head.Kind] = &ResourceList{
				Kind:      r.Head.Kind,
				Mapping:   r.Info.ResourceMapping().Resource,
				Resources: []string{},
			}
		}
		res[r.Head.Kind].Resources = append(res[r.Head.Kind].Resources, r.Name)
	}

	return res
}

// getOldResourceMap fetches the last set of resources deployed into the namespace from
// `resourceSecretName` secret.
func getOldResourceMap(infoGen resourceutil.InfoGenerator, namespace string) (map[string]*ResourceList, error) {
	secret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceSecretName,
			Namespace: namespace,
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
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

	remoteObject, err := helper.Get(namespace, resourceSecretName)

	if err != nil {
		if apierrors.IsNotFound(err) {
			return map[string]*ResourceList{}, nil
		}
		return nil, err
	}

	uncastVersionedObj, err := scheme.Scheme.ConvertToVersion(remoteObject, apiv1.SchemeGroupVersion)
	if err != nil {
		return nil, err
	}
	remoteSecret, ok := uncastVersionedObj.(*apiv1.Secret)
	if !ok {
		return nil, fmt.Errorf("Error in conversion to Cronjob")
	}

	res := make(map[string]*ResourceList)

	err = json.Unmarshal(remoteSecret.Data[resourceField], &res)

	if err != nil {
		return nil, err
	}

	if len(res) == 0 {
		return nil, errors.New("Resource field is empty")
	}

	return res, nil
}

// deletedResources returns the resources not contained in the last deploy
func deletedResources(actual, old map[string]*ResourceList) map[string]*ResourceList {
	res := make(map[string]*ResourceList)

	// get diff on already existing resources, the new ones
	// are added with the new secret.
	for key := range old {
		if _, ok := res[key]; !ok {
			res[key] = &ResourceList{
				Kind:    old[key].Kind,
				Mapping: old[key].Mapping,
			}
		}

		if _, ok := actual[key]; ok {
			res[key].Resources = diffResourceArray(actual[key].Resources, old[key].Resources)
		} else {
			res[key].Resources = old[key].Resources
		}
	}

	// Remove entries with empty diff
	for kind, resourceGroup := range res {
		if len(resourceGroup.Resources) == 0 {
			delete(res, kind)
		}
	}

	return res
}

// diffResourceArray returns the old values missing in the new slice
func diffResourceArray(actual, old []string) []string {
	res := []string{}

	for _, oValue := range old {
		if !contains(actual, oValue) {
			res = append(res, oValue)
		}
	}

	return res
}

// contains takes a string slice and search for an element in it.
func contains(res []string, s string) bool {
	for _, item := range res {
		if item == s {
			return true
		}
	}
	return false
}
