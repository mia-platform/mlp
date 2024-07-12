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
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	jpltesting "github.com/mia-platform/jpl/pkg/testing"
	"github.com/mia-platform/jpl/pkg/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	fcv1beta3 "k8s.io/api/flowcontrol/v1beta3"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/resource"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	restfake "k8s.io/client-go/rest/fake"
	"k8s.io/utils/clock"
	clocktesting "k8s.io/utils/clock/testing"
)

func TestCommand(t *testing.T) {
	t.Parallel()

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Log(r.Method, r.URL.Path)
		switch r.URL.Path {
		case "/livez/ping":
			w.WriteHeader(http.StatusOK)
			w.Header().Add(fcv1beta3.ResponseHeaderMatchedFlowSchemaUID, "unused")
		default:
			for key, values := range jpltesting.DefaultHeaders() {
				for _, v := range values {
					w.Header().Add(key, v)
				}
			}
			w.Write([]byte("{}"))
		}
	}))
	defer testServer.Close()

	flags := genericclioptions.NewConfigFlags(false)
	flags.APIServer = &testServer.URL

	cmd := NewCommand(flags)
	assert.NotNil(t, cmd)

	cmd.SetArgs([]string{
		"--filename=-",
		"--namespace=mlp-test-deploy",
	})
	cmd.Execute()
}

func TestOptions(t *testing.T) {
	t.Parallel()

	reader := new(bytes.Reader)
	buffer := new(bytes.Buffer)
	configFlags := genericclioptions.NewConfigFlags(false)

	expectedOpts := &Options{
		inputPaths:    []string{"input"},
		deployType:    "smart_deploy",
		reader:        reader,
		writer:        buffer,
		clientFactory: util.NewFactory(configFlags),
		clock:         clock.RealClock{},
	}

	flag := &Flags{
		inputPaths: []string{"input"},
		deployType: "smart_deploy",
	}
	_, err := flag.ToOptions(reader, buffer)
	assert.ErrorContains(t, err, "config flags are required")

	flag.ConfigFlags = configFlags
	opts, err := flag.ToOptions(reader, buffer)
	require.NoError(t, err)

	assert.Equal(t, expectedOpts, opts)
	assert.NoError(t, opts.Validate())

	opts.deployType = "wrong"
	assert.ErrorContains(t, opts.Validate(), `invalid deploy type value: "wrong"`)
	opts.deployType = "deploy_all"

	opts.inputPaths = []string{}
	assert.ErrorContains(t, opts.Validate(), "at least one path must be specified")

	opts.inputPaths = []string{"input", stdinToken}
	assert.ErrorContains(t, opts.Validate(), "cannot read from stdin and other paths together")
}

func TestRun(t *testing.T) {
	t.Parallel()
	testdata := "testdata"
	namespace := "mlp-deploy-test"
	fakeClock := clocktesting.NewFakePassiveClock(time.Date(1970, time.January, 0, 0, 0, 0, 0, time.UTC))

	secret := jpltesting.UnstructuredFromFile(t, filepath.Join(testdata, "resources", "secret.yaml"))
	fakeDynamicClient := dynamicfake.NewSimpleDynamicClient(jpltesting.Scheme, secret)

	tests := map[string]struct {
		options             *Options
		expectedResources   []*resourceValidation
		timeout             time.Duration
		expectedCallsNumber int
		expectedError       string
	}{
		"apply objects": {
			options: &Options{
				inputPaths: []string{filepath.Join(testdata, "resources")},
				deployType: "deploy_all",
				dryRun:     true,
				clock:      fakeClock,
			},
			timeout: 1 * time.Second,
			expectedResources: []*resourceValidation{
				{
					path:             fmt.Sprintf("/namespaces/%s/configmaps/example", namespace),
					method:           http.MethodPatch,
					expectedFilePath: filepath.Join(testdata, "expectations", "configmap.yaml"),
				},
				{
					path:             fmt.Sprintf("/namespaces/%s/deployments/example", namespace),
					method:           http.MethodPatch,
					expectedFilePath: filepath.Join(testdata, "expectations", "deployment.yaml"),
				},
				{
					path:             fmt.Sprintf("/namespaces/%s/cronjobs/example", namespace),
					method:           http.MethodPatch,
					expectedFilePath: filepath.Join(testdata, "expectations", "cronjob.yaml"),
				},
				{
					path:             fmt.Sprintf("/namespaces/%s/jobs/example", namespace),
					method:           http.MethodPatch,
					expectedFilePath: filepath.Join(testdata, "expectations", "job.yaml"),
				},
			},
			expectedCallsNumber: 7,
		},
		"error reading files": {
			options: &Options{
				inputPaths: []string{filepath.Join(testdata, "missing.yaml")},
				deployType: "deploy_all",
				dryRun:     true,
				clock:      fakeClock,
			},
			timeout:             1 * time.Second,
			expectedResources:   []*resourceValidation{},
			expectedCallsNumber: 0,
			expectedError:       fmt.Sprintf("fail to read from path %q", filepath.Join(testdata, "missing.yaml")),
		},
		"error with timeout context": {
			options: &Options{
				inputPaths: []string{filepath.Join(testdata, "resources")},
				deployType: "deploy_all",
				dryRun:     true,
				clock:      fakeClock,
			},
			timeout:             0 * time.Millisecond,
			expectedResources:   []*resourceValidation{},
			expectedCallsNumber: 0,
			expectedError:       "context deadline exceeded",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			stringBuilder := new(strings.Builder)
			ctx, cancel := context.WithTimeout(context.TODO(), test.timeout)
			defer cancel()

			callsCounter := 0
			tf := jpltesting.NewTestClientFactory().
				WithNamespace(namespace)
			tf.Client = &restfake.RESTClient{
				NegotiatedSerializer: resource.UnstructuredPlusDefaultContentConfig().NegotiatedSerializer,
				Client: restfake.CreateHTTPClient(func(r *http.Request) (*http.Response, error) {
					res, err := validationRoundTripper(t, test.expectedResources, r)
					if err == nil {
						callsCounter++
					}
					return res, err
				}),
			}
			tf.FakeDynamicClient = fakeDynamicClient

			test.options.clientFactory = tf
			test.options.writer = stringBuilder

			err := test.options.Run(ctx)
			t.Log(stringBuilder.String())

			assert.Equal(t, test.expectedCallsNumber, callsCounter)
			switch len(test.expectedError) {
			case 0:
				require.NoError(t, err)
			default:
				assert.ErrorContains(t, err, test.expectedError)
			}
		})
	}
}

func validationRoundTripper(t *testing.T, resources []*resourceValidation, r *http.Request) (*http.Response, error) {
	t.Helper()
	path := r.URL.Path
	method := r.Method
	inventoryPath := fmt.Sprintf("/api/v1/namespaces/mlp-deploy-test/configmaps/%s", inventoryName)
	oldInventoryPath := fmt.Sprintf("/api/v1/namespaces/mlp-deploy-test/secrets/%s", oldInventoryName)
	codec := jpltesting.Codecs.LegacyCodec(jpltesting.Scheme.PrioritizedVersionsAllGroups()...)

	if r.Body != nil {
		defer r.Body.Close()
	}

	switch {
	case path == inventoryPath && method == http.MethodGet:
		return &http.Response{StatusCode: http.StatusNotFound, Header: jpltesting.DefaultHeaders()}, nil
	case path == oldInventoryPath && method == http.MethodGet:
		list := &resourceList{
			Gvk:       corev1.SchemeGroupVersion.WithKind(reflect.TypeOf(corev1.Secret{}).Name()),
			Resources: []string{"example"},
		}
		data, err := json.Marshal(map[string]*resourceList{
			"Secret": list,
		})
		require.NoError(t, err)
		sec := &corev1.Secret{Data: map[string][]byte{
			oldInventoryKey: data,
		}}
		body := io.NopCloser(bytes.NewReader([]byte(runtime.EncodeOrDie(codec, sec))))
		return &http.Response{StatusCode: http.StatusOK, Body: body, Header: jpltesting.DefaultHeaders()}, nil
	case path == inventoryPath && method == http.MethodPatch:
		bodyData, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		cm := new(corev1.ConfigMap)
		require.NoError(t, runtime.DecodeInto(codec, bodyData, cm))
		assert.Equal(t, 5, len(cm.Data))
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     jpltesting.DefaultHeaders(),
			Body:       io.NopCloser(bytes.NewReader(bodyData)),
		}, nil
	}

	for _, res := range resources {
		if !res.canValidateRequest(t, r) {
			continue
		}

		body := res.validateBody(t, r.Body)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: jpltesting.DefaultHeaders()}, nil
	}

	return nil, fmt.Errorf("unexpected call: %q, method %s", path, method)
}

type resourceValidation struct {
	expectedFilePath string
	method           string
	path             string
}

func (rv *resourceValidation) canValidateRequest(t *testing.T, r *http.Request) bool {
	t.Helper()
	return strings.HasPrefix(r.URL.Path, rv.path) && r.Method == rv.method
}

func (rv *resourceValidation) validateBody(t *testing.T, body io.ReadCloser) []byte {
	t.Helper()

	bodyData, err := io.ReadAll(body)
	require.NoError(t, err)

	obj := new(unstructured.Unstructured)
	decoder := jpltesting.Codecs.UniversalDecoder()
	err = runtime.DecodeInto(decoder, bodyData, obj)
	require.NoError(t, err)

	expected := jpltesting.UnstructuredFromFile(t, rv.expectedFilePath)
	if obj.GetKind() == "Job" {
		if _, found := obj.GetAnnotations()["cronjob.kubernetes.io/instantiate"]; found {
			obj.SetName(expected.GetName())
		}
	}

	require.Equal(t, expected, obj)

	return bodyData
}
