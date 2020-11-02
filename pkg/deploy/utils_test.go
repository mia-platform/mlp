package deploy

import (
	"reflect"
	"testing"

	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/resourceutil"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/resource"
)

func TestMakeResourceMap(t *testing.T) {

	gvk := schema.GroupVersionKind{
		Group:   "group",
		Version: "version",
		Kind:    "Bar",
	}

	gvr := schema.GroupVersionResource{
		Group:    "group",
		Version:  "version",
		Resource: "bars",
	}

	testcases := []struct {
		description string
		old         resourceutil.Resource
		expected    map[string]*ResourceList
	}{
		{
			description: "Create map from resourceutil.Resource",
			old: resourceutil.Resource{
				Head: resourceutil.ResourceHead{
					Kind: gvk.Kind,
				},
				Info: &resource.Info{
					Mapping: &meta.RESTMapping{
						GroupVersionKind: gvk,
						Resource:         gvr,
					},
				},
				Name: "foo",
			},
			expected: map[string]*ResourceList{"Bar": &ResourceList{Kind: "Bar", Mapping: gvr, Resources: []string{"foo"}}},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			actual := makeResourceMap([]resourceutil.Resource{tt.old})
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestGetOldResourceMap(t *testing.T) {

	testcases := []struct {
		description string
		old         *apiv1.Secret
		expected    map[string]*ResourceList
		error       func(t *testing.T, err error)
	}{
		{
			description: "resources field is unmarshaled correctly",
			old: &apiv1.Secret{
				Data: map[string][]byte{"resources": []byte(`{"secrets":{"resources":["foo", "bar"]}, "configmaps": {"resources":[]}}`)},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceSecretName,
					Namespace: "foo",
				},
				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			},
			expected: map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo", "bar"}}, "configmaps": &ResourceList{Resources: []string{}}},
			error: func(t *testing.T, err error) {
				require.Nil(t, err)
			},
		},
		{
			description: "resource field is empty",
			old: &apiv1.Secret{
				Data: map[string][]byte{"resources": []byte(`{}`)},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceSecretName,
					Namespace: "foo",
				},
				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			},
			expected: nil,
			error: func(t *testing.T, err error) {
				require.NotNil(t, err)
				require.EqualError(t, err, "Resource field is empty")
			},
		},
		{
			description: "resource field does not contain map[string][]string but map[string]string ",
			old: &apiv1.Secret{
				Data: map[string][]byte{"resources": []byte(`{ "foo": "bar" `)},
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceSecretName,
					Namespace: "foo",
				},
				TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
			},
			expected: nil,
			error: func(t *testing.T, err error) {
				require.NotNil(t, err)
			},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			builder := resourceutil.NewFakeBuilder()
			err := builder.AddResources([]runtime.Object{tt.old})
			require.Nil(t, err)
			actual, err := getOldResourceMap(builder, "foo")
			require.Equal(t, tt.expected, actual)

			tt.error(t, err)
		})
	}
}

func TestDeletedResources(t *testing.T) {
	testcases := []struct {
		description string
		old         map[string]*ResourceList
		new         map[string]*ResourceList
		expected    map[string]*ResourceList
	}{
		{
			description: "No diff with equal maps",
			old:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo", "bar"}}},
			new:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo", "bar"}}},
			expected:    map[string]*ResourceList{},
		},
		{
			description: "Expected old map if new is empty",
			old:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo", "bar"}}},
			new:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{}}},
			expected:    map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo", "bar"}}},
		},
		{
			description: "Remove one resource from resourceList",
			old:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo", "bar"}}},
			new:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo"}}},
			expected:    map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"bar"}}},
		},
		{
			description: "Add one resource type",
			old:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo", "bar"}}},
			new:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo"}}, "configmaps": &ResourceList{Resources: []string{"foo"}}},
			expected:    map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"bar"}}},
		},
		{
			description: "Delete one resource type",
			old:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo"}}, "configmaps": &ResourceList{Resources: []string{"foo"}}},
			new:         map[string]*ResourceList{"secrets": &ResourceList{Resources: []string{"foo"}}},
			expected:    map[string]*ResourceList{"configmaps": &ResourceList{Resources: []string{"foo"}}},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			actual := deletedResources(tt.new, tt.old)
			require.True(t, reflect.DeepEqual(tt.expected, actual))
		})
	}
}

func TestDiffResourceArray(t *testing.T) {
	testcases := []struct {
		description string
		old         []string
		new         []string
		expected    []string
	}{
		{
			description: "No diff with equal slices",
			old:         []string{"foo", "bar"},
			new:         []string{"foo", "bar"},
			expected:    []string{},
		},
		{
			description: "Expected old array if new is empty",
			old:         []string{"foo", "bar"},
			new:         []string{},
			expected:    []string{"foo", "bar"},
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			actual := diffResourceArray(tt.new, tt.old)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func TestContains(t *testing.T) {
	testcases := []struct {
		description string
		array       []string
		element     string
		expected    bool
	}{
		{
			description: "the element is contained in the slice",
			array:       []string{"foo", "bar"},
			element:     "foo",
			expected:    true,
		},
		{
			description: "the element is not contained in the slice",
			array:       []string{"foo", "bar"},
			element:     "foobar",
			expected:    false,
		},
		{
			description: "the element is not contained in empty slice",
			array:       []string{},
			element:     "foobar",
			expected:    false,
		},
	}

	for _, tt := range testcases {
		t.Run(tt.description, func(t *testing.T) {
			actual := contains(tt.array, tt.element)
			require.Equal(t, tt.expected, actual)
		})
	}
}
