package resourceutil

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	apiv1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestFromFunctions(t *testing.T) {
	b := NewFakeBuilder()
	secret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}
	addToHelper := false
	b.AddResources([]runtime.Object{secret}, addToHelper)

	t.Run("From Stream return the first element of ClusterObjs", func(t *testing.T) {
		buf, err := yaml.Marshal(secret)
		require.Nil(t, err)

		info, err := b.FromStream(bytes.NewBuffer(buf))
		require.Nil(t, err)
		require.Equal(t, secret.Name, info[0].Name, "Object info and secret should have the same Name")
		require.Equal(t, secret.Namespace, info[0].Namespace, "Object info and secret should have the same Namespace")
		require.Equal(t, secret, info[0].Object, "Object and secret should be the same")
	})

	t.Run("From Names return the first element of ClusterObjs", func(t *testing.T) {
		info, err := b.FromNames("bar", "foobar", []string{"foo"})
		require.Nil(t, err)
		require.Equal(t, secret.Name, info[0].Name, "Object info and secret should have the same Name")
		require.Equal(t, secret.Namespace, info[0].Namespace, "Object info and secret should have the same Namespace")
		require.Equal(t, secret, info[0].Object, "Object and secret should be the same")
	})

	t.Run("From File return the first element of ClusterObjs", func(t *testing.T) {
		info, err := b.FromFile("testdata/kubernetesersource.yaml")
		require.Nil(t, err)
		require.Equal(t, secret.Name, info[0].Name, "Object info and secret should have the same Name")
		require.Equal(t, secret.Namespace, info[0].Namespace, "Object info and secret should have the same Namespace")
		require.Equal(t, secret, info[0].Object, "Object and secret should be the same")
	})

	t.Run("From File returns an empty array of length > 1 if multiple resource in file", func(t *testing.T) {
		info, err := b.FromFile("testdata/tworesources.yaml")
		require.Nil(t, err)
		require.Equal(t, 2, len(info), "Info length should be greater than 1")
	})
}

func TestAddResources(t *testing.T) {
	t.Run("Add resources to builder and helper", func(t *testing.T) {
		b := NewFakeBuilder()
		secret := &apiv1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		addResourceToHelper := true
		b.AddResources([]runtime.Object{secret}, addResourceToHelper)
		require.Equal(t, secret, b.Helper.ClusterObjs[0].obj, "the object should be in ClusterObjs")
		require.Equal(t, secret, b.Resources[0].obj, "the object should be in ClusterObjs")
	})
	t.Run("Add resources to builder", func(t *testing.T) {
		b := NewFakeBuilder()
		secret := &apiv1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		addResourceToHelper := false
		b.AddResources([]runtime.Object{secret}, addResourceToHelper)
		require.Equal(t, 0, len(b.Helper.ClusterObjs), "Helper resources should be empty")
		require.Equal(t, secret, b.Resources[0].obj, "the object should be in ClusterObjs")
	})
}

func TestNewHelper(t *testing.T) {
	t.Run("the helper returned has the flags set to false", func(t *testing.T) {
		b := NewFakeBuilder()
		b.Helper.CreateCalled = true
		b.Helper.PatchCalled = true
		b.Helper.ReplaceCalled = true
		_ = b.NewHelper(nil, nil)
		require.False(t, b.Helper.CreateCalled)
		require.False(t, b.Helper.PatchCalled)
		require.False(t, b.Helper.ReplaceCalled)
	})
}

func TestGetHelper(t *testing.T) {
	t.Run("helper contains the resource added", func(t *testing.T) {
		b := NewFakeBuilder()
		secret := &apiv1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		b.AddResources([]runtime.Object{secret}, true)
		helper := b.NewHelper(nil, nil)
		obj, err := helper.Get("bar", "foo")
		require.Nil(t, err)
		require.Equal(t, secret, obj, "the object should be in ClusterObjs")
	})
	t.Run("helper does not contain the resource", func(t *testing.T) {
		b := NewFakeBuilder()
		helper := b.NewHelper(nil, nil)
		obj, err := helper.Get("bar", "foo")
		require.True(t, apierrors.IsNotFound(err), "the helper does not contain the object")
		require.Nil(t, obj)
	})
	t.Run("helper does not contain the resource if addToHelper is false", func(t *testing.T) {
		b := NewFakeBuilder()
		secret := &apiv1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
		}
		b.AddResources([]runtime.Object{secret}, false)
		helper := b.NewHelper(nil, nil)
		obj, err := helper.Get("bar", "foo")
		require.True(t, apierrors.IsNotFound(err), "the helper does not contain the object")
		require.Nil(t, obj)
	})
}

func TestCreateHelper(t *testing.T) {
	secret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	t.Run("helper creates the resource", func(t *testing.T) {
		b := NewFakeBuilder()
		helper := b.NewHelper(nil, nil)
		obj, err := helper.Create("bar", true, secret)
		require.Nil(t, err)
		require.Equal(t, secret, obj, "the object should be in ClusterObjs")
	})
	t.Run("helper returns error if the resource already exists", func(t *testing.T) {
		b := NewFakeBuilder()
		b.AddResources([]runtime.Object{secret}, true)
		helper := b.NewHelper(nil, nil)
		_, err := helper.Create("bar", true, secret)
		require.True(t, apierrors.IsAlreadyExists(err))
	})
}

func TestReplaceHelper(t *testing.T) {
	oldSecret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	newSecret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			Annotations: map[string]string{
				"new": "true",
			},
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	t.Run("helper replace the resource", func(t *testing.T) {
		b := NewFakeBuilder()
		b.AddResources([]runtime.Object{oldSecret}, true)
		helper := b.NewHelper(nil, nil)
		obj, err := helper.Replace("bar", "foo", true, newSecret)
		require.Nil(t, err)
		require.Equal(t, newSecret, obj, "the object should be in ClusterObjs")
	})
	t.Run("helper returns error if the resource is not found", func(t *testing.T) {
		b := NewFakeBuilder()
		helper := b.NewHelper(nil, nil)
		_, err := helper.Replace("bar", "foo", true, newSecret)
		require.True(t, apierrors.IsNotFound(err))
	})
}

func TestDeleteHelper(t *testing.T) {
	oldSecret := &apiv1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	t.Run("helper deletes the resource", func(t *testing.T) {
		b := NewFakeBuilder()
		b.AddResources([]runtime.Object{oldSecret}, true)
		helper := b.NewHelper(nil, nil)
		obj, err := helper.Delete("bar", "foo")
		require.Nil(t, err)
		require.Equal(t, 0, len(b.Helper.ClusterObjs), "the resources should be empty")
		require.Equal(t, oldSecret, obj, "the object should be in ClusterObjs")
	})
	t.Run("helper returns error if the resource is not found", func(t *testing.T) {
		b := NewFakeBuilder()
		helper := b.NewHelper(nil, nil)
		_, err := helper.Delete("bar", "foo")
		require.True(t, apierrors.IsNotFound(err))
	})
}
