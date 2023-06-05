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

package generate

import (
	"path/filepath"
	"testing"

	"github.com/mia-platform/mlp/internal/utils"
	"github.com/stretchr/testify/require"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testdata = "testdata/"

func TestTLSSecretGenerate(t *testing.T) {
	certPath := filepath.Join(testdata, "cert.pem")
	cert, _ := utils.ReadFile(certPath)

	s := secret{
		Name: "foo",
		When: "always",
		TLS: tls{
			Cert: tlsData{
				From: "file",
				File: certPath,
			},
			Key: tlsData{
				From:  "literal",
				Value: "key",
			},
		},
	}

	expected := &apiv1.Secret{
		Data: map[string][]byte{
			"tls.crt": cert,
			"tls.key": []byte("key"),
		},
		Type: apiv1.SecretTypeTLS,
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Annotations: map[string]string{
				"mia-platform.eu/deploy": "always",
			},
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	require.Equal(t, expected, generateSecret(s), "they should be equal")
}

func TestDockerSecretGenerate(t *testing.T) {
	s := secret{
		Name: "foo",
		When: "always",
		Docker: docker{
			Email:    "fooEmail",
			Password: "fooPass",
			Server:   "fooServ",
			Username: "fooUser",
		},
	}
	expected := &apiv1.Secret{
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"fooServ":{"username":"fooUser","password":"fooPass","email":"fooEmail","auth":"Zm9vVXNlcjpmb29QYXNz"}}}`),
		},
		Type: apiv1.SecretTypeDockerConfigJson,
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Annotations: map[string]string{
				"mia-platform.eu/deploy": "always",
			},
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	require.Equal(t, expected, generateSecret(s), "they should be equal")
}

func TestOpaqueSecretGenerate(t *testing.T) {
	keyPath := filepath.Join(testdata, "key.pem")
	key, _ := utils.ReadFile(keyPath)

	secrets := []secret{
		{
			Name: "foo",
			When: "once",
			Data: data{
				{
					From: "file",
					File: keyPath,
				},
				{
					From:  "literal",
					Key:   "foo",
					Value: "foo",
				},
			},
		},
	}
	expected := &apiv1.Secret{
		Data: map[string][]byte{
			"key.pem": key,
			"foo":     []byte("foo"),
		},
		Type: apiv1.SecretTypeOpaque,
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
			Annotations: map[string]string{
				"mia-platform.eu/deploy": "once",
			},
		},
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"},
	}

	for _, s := range secrets {
		require.Equal(t, expected, generateSecret(s), "they should be equal")
	}
}

func TestConfigMap(t *testing.T) {
	keyPath := filepath.Join(testdata, "key.pem")
	key, _ := utils.ReadFile(keyPath)

	cfs := []configMap{
		{
			Name: "foo",
			Data: data{
				{
					From: "file",
					File: keyPath,
				},
				{
					From:  "literal",
					Key:   "foo",
					Value: "foo",
				},
			},
		},
	}
	expected := &apiv1.ConfigMap{
		Data: map[string]string{
			"key.pem": string(key),
			"foo":     "foo",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		TypeMeta:   metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"},
	}

	for _, cf := range cfs {
		require.Equal(t, expected, generateConfigMap(cf), "they should be equal")
	}
}

func TestGenerate(t *testing.T) {
	configurationPath := filepath.Join(testdata, "configuration.yml")
	data, err := Generate(configurationPath, []string{})

	require.Nil(t, err, "No error when function return corrrectly")
	require.NotNil(t, data, "Data must be not nil")
	require.Equal(t, 5, len(data), "must contains 5 elements")
}
