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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"testing"

	jplresource "github.com/mia-platform/jpl/pkg/resource"
	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	restfake "k8s.io/client-go/rest/fake"
)

func TestLoadInventory(t *testing.T) { //nolint: gocyclo
	t.Parallel()

	namespace := "test-inventory"
	configMapPath := fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", namespace, inventoryName)
	secretPath := fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", namespace, oldInventoryName)
	deployResource := jplresource.ObjectMetadata{
		Kind:      "Deployment",
		Group:     "apps",
		Name:      "example",
		Namespace: namespace,
	}
	namespaceResource := jplresource.ObjectMetadata{
		Kind:      "Namespace",
		Group:     "",
		Name:      "example",
		Namespace: "",
	}
	codec := jpltesting.Codecs.LegacyCodec(jpltesting.Scheme.PrioritizedVersionsAllGroups()...)

	tests := map[string]struct {
		client        *http.Client
		expectedSet   sets.Set[jplresource.ObjectMetadata]
		expectedError string
	}{
		"compatibility mode": {
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				switch {
				case path == configMapPath && method == http.MethodGet:
					return &http.Response{StatusCode: http.StatusNotFound, Header: jpltesting.DefaultHeaders()}, nil
				case path == secretPath && method == http.MethodGet:
					data, err := json.Marshal(map[string]*resourceList{
						"Deployment": {
							Gvk:       appsv1.SchemeGroupVersion.WithKind(reflect.TypeOf(appsv1.Deployment{}).Name()),
							Resources: []string{"example"},
						},
						"Namespace": {
							Gvk:       corev1.SchemeGroupVersion.WithKind(reflect.TypeOf(corev1.Namespace{}).Name()),
							Resources: []string{"example"},
						},
					})
					require.NoError(t, err)
					sec := &corev1.Secret{Data: map[string][]byte{
						oldInventoryKey: data,
					}}
					body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(codec, sec))))
					return &http.Response{StatusCode: http.StatusOK, Body: body, Header: jpltesting.DefaultHeaders()}, nil
				}

				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
			expectedSet: sets.New(
				deployResource,
				namespaceResource,
			),
		},
		"compatibility mode old version": {
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				switch {
				case path == configMapPath && method == http.MethodGet:
					return &http.Response{StatusCode: http.StatusNotFound, Header: jpltesting.DefaultHeaders()}, nil
				case path == secretPath && method == http.MethodGet:
					data, err := json.Marshal(map[string]*oldResourceList{
						"Deployment": {
							Kind: "Deployment",
							Mapping: schema.GroupVersionResource{
								Group:    "apps",
								Version:  "v1",
								Resource: "deployments",
							},
							Resources: []string{"example"},
						},
						"Namespace": {
							Kind: "Namespace",
							Mapping: schema.GroupVersionResource{
								Group:    "",
								Version:  "v1",
								Resource: "namespaces",
							},
							Resources: []string{"example"},
						},
					})
					require.NoError(t, err)
					sec := &corev1.Secret{Data: map[string][]byte{
						oldInventoryKey: data,
					}}
					body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(codec, sec))))
					return &http.Response{StatusCode: http.StatusOK, Body: body, Header: jpltesting.DefaultHeaders()}, nil
				}

				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
			expectedSet: sets.New(
				deployResource,
				namespaceResource,
			),
		},
		"new mode": {
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				if r.URL.Path == configMapPath && r.Method == http.MethodGet {
					cm := &corev1.ConfigMap{
						Data: map[string]string{
							namespaceResource.ToString(): "",
							deployResource.ToString():    "",
						},
					}
					body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(codec, cm))))
					return &http.Response{StatusCode: http.StatusOK, Body: body, Header: jpltesting.DefaultHeaders()}, nil
				}

				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
			expectedSet: sets.New(
				deployResource,
				namespaceResource,
			),
		},
		"both inventory empty": {
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				switch {
				case path == configMapPath && method == http.MethodGet:
					return &http.Response{StatusCode: http.StatusNotFound, Header: jpltesting.DefaultHeaders()}, nil
				case path == secretPath && method == http.MethodGet:
					return &http.Response{StatusCode: http.StatusNotFound, Header: jpltesting.DefaultHeaders()}, nil
				}
				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
			expectedSet: make(sets.Set[jplresource.ObjectMetadata]),
		},
		"error in new mode": {
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
			expectedError: fmt.Sprintf("unexpected call: %q, method GET", configMapPath),
		},
		"error in compatibility mode": {
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				if r.URL.Path == configMapPath && r.Method == http.MethodGet {
					return &http.Response{StatusCode: http.StatusNotFound, Header: jpltesting.DefaultHeaders()}, nil
				}
				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
			expectedError: fmt.Sprintf("unexpected call: %q, method GET", secretPath),
		},
		"unavailable kind in compatibility mode": {
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				switch {
				case path == configMapPath && method == http.MethodGet:
					return &http.Response{StatusCode: http.StatusNotFound, Header: jpltesting.DefaultHeaders()}, nil
				case path == secretPath && method == http.MethodGet:
					data, err := json.Marshal(map[string]*oldResourceList{
						"Deployment": {
							Kind: "Deployment",
							Mapping: schema.GroupVersionResource{
								Group:    "apps",
								Version:  "v1",
								Resource: "deployments",
							},
							Resources: []string{"example"},
						},
						"Namespace": {
							Kind: "Namespace",
							Mapping: schema.GroupVersionResource{
								Group:    "",
								Version:  "v1",
								Resource: "namespaces",
							},
							Resources: []string{"example"},
						},
						"Foo": {
							Kind: "Foo",
							Mapping: schema.GroupVersionResource{
								Group:    "example.com",
								Version:  "v1alpha1",
								Resource: "foos",
							},
						},
					})
					require.NoError(t, err)
					sec := &corev1.Secret{Data: map[string][]byte{
						oldInventoryKey: data,
					}}
					body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(codec, sec))))
					return &http.Response{StatusCode: http.StatusOK, Body: body, Header: jpltesting.DefaultHeaders()}, nil
				}

				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
			expectedSet: sets.New(
				deployResource,
				namespaceResource,
			),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			factory := jpltesting.NewTestClientFactory()
			factory.Client = &restfake.RESTClient{
				Client: test.client,
			}

			inv, err := NewInventory(factory, inventoryName, namespace, "mlp")
			require.NoError(t, err)

			set, err := inv.Load(context.TODO())
			switch len(test.expectedError) {
			case 0:
				assert.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
			assert.Equal(t, test.expectedSet, set)
		})
	}
}

func TestSavingInventory(t *testing.T) {
	t.Parallel()

	namespace := "test-inventory"
	configMapPath := fmt.Sprintf("/api/v1/namespaces/%s/configmaps/%s", namespace, inventoryName)
	secretPath := fmt.Sprintf("/api/v1/namespaces/%s/secrets/%s", namespace, oldInventoryName)

	tests := map[string]struct {
		client            *http.Client
		dryRun            bool
		compatibilityMode bool
		expectedError     string
	}{
		"don't call secret deletion if dryRun": {
			dryRun:            true,
			compatibilityMode: true,
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				if path == configMapPath && method == http.MethodPatch {
					return &http.Response{StatusCode: http.StatusOK, Body: r.Body, Header: jpltesting.DefaultHeaders()}, nil
				}
				return nil, fmt.Errorf("unexpected call: %q, method %s", r.URL.Path, r.Method)
			}),
		},
		"call secret deletion after inventory if compatibility mode is enabled": {
			compatibilityMode: true,
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				switch {
				case path == configMapPath && method == http.MethodPatch:
					return &http.Response{StatusCode: http.StatusOK, Body: r.Body, Header: jpltesting.DefaultHeaders()}, nil
				case path == secretPath && method == http.MethodDelete:
					return &http.Response{StatusCode: http.StatusOK, Header: jpltesting.DefaultHeaders()}, nil
				}
				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
		},
		"don't call delete old inventory if not in compatibility mode": {
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				if path == configMapPath && method == http.MethodPatch {
					return &http.Response{StatusCode: http.StatusOK, Body: r.Body, Header: jpltesting.DefaultHeaders()}, nil
				}
				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
		},
		"error in saving configmap inventory don't trigger delete secret": {
			dryRun:            true,
			compatibilityMode: true,
			client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
				path := r.URL.Path
				method := r.Method
				if path == configMapPath && method == http.MethodPatch {
					return &http.Response{StatusCode: http.StatusForbidden, Body: r.Body, Header: jpltesting.DefaultHeaders()}, nil
				}
				return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
			}),
			expectedError: "failed to save inventory",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			factory := jpltesting.NewTestClientFactory()
			factory.Client = &restfake.RESTClient{
				Client: test.client,
			}

			inv, err := NewInventory(factory, inventoryName, namespace, "mlp")
			castedInventory, ok := inv.(*Inventory)
			require.True(t, ok)
			castedInventory.compatibilityMode = test.compatibilityMode
			require.NoError(t, err)

			err = inv.Save(context.TODO(), test.dryRun)
			if len(test.expectedError) > 0 {
				assert.ErrorContains(t, err, test.expectedError)
				return
			}

			assert.NoError(t, err)
		})
	}
}
