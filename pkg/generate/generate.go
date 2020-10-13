// Copyright 2020 Mia srl
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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path"

	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/interpolate"
	"github.com/google/go-cmp/cmp"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"
)

// Run execute the generate command from cli
func Run(configPath []string, prefixes []string, outputPath string) {
	fileNames, err := utils.ExtractYAMLFiles(configPath)
	utils.CheckError(err)

	for _, filePath := range fileNames {

		data, err := Generate(filePath, prefixes)
		utils.CheckError(err)

		utils.WriteYamlsToDisk(data, outputPath)
	}
}

// Generate Secrets and ConfigMaps files from a configuration file
func Generate(configuration string, prefixes []string) (map[string]runtime.Object, error) {

	rawConfiguration, err := utils.ReadFile(configuration)
	if err != nil {
		return nil, err
	}

	interpolatedData, err := interpolate.Interpolate(rawConfiguration, prefixes, "\\{\\{([A-Z0-9_]+)\\}\\}")
	if err != nil {
		return nil, err
	}

	configurationData := generateConfiguration{}
	err = yaml.Unmarshal(interpolatedData, &configurationData)
	if err != nil {
		return nil, err
	}

	resources := make(map[string]runtime.Object)

	for _, secret := range configurationData.Secret {
		object := generateSecret(secret)
		fileName := fmt.Sprintf("%s.secret", secret.Name)
		resources[fileName] = object
	}

	for _, configmap := range configurationData.ConfigMap {
		object := generateConfigMap(configmap)
		fileName := fmt.Sprintf("%s.configmap", configmap.Name)
		resources[fileName] = object
	}

	return resources, nil
}

// keyValueFromData creates the content of secret data field
// based on data type which can be `literal` or `file`
func keyValueFromData(data data) map[string][]byte {
	keyValue := make(map[string][]byte)

	for _, d := range data {
		if d.From == "literal" {
			keyValue[d.Key] = []byte(d.Value)
		} else if d.From == "file" {
			baseName := path.Base(d.File)
			fileContent, err := utils.ReadFile(d.File)
			utils.CheckError(err)
			keyValue[baseName] = fileContent
		}
	}
	return keyValue
}

// genDockercfg returns base64 encoded .dockercfg string
func genDockercfg(docker docker) []byte {
	userPassString := fmt.Sprintf("%s:%s", docker.Username, docker.Password)
	userPassBase64 := base64.StdEncoding.EncodeToString([]byte(userPassString))
	cfg := dockerCfg{dockerConfig{docker.Server: dockerConfigEntry{Username: docker.Username, Password: docker.Password, Email: docker.Email, Auth: userPassBase64}}}
	res, err := json.Marshal(cfg)
	utils.CheckError(err)
	return res
}

// readTLSData reads the TLS secret data from file or from literal
func readTLSData(d tlsData) (out []byte) {
	switch d.From {
	case "literal":
		return []byte(d.Value)
	case "file":
		fileContent, err := utils.ReadFile(d.File)
		utils.CheckError(err)
		return fileContent
	default:
		panic("unknown data source:" + d.From)
	}
}

// generateConfigMap generates a configmap starting from configurations stored in `Configmap` struct
func generateConfigMap(cm configMap) *apiv1.ConfigMap {
	object := metav1.ObjectMeta{Name: cm.Name}
	typeMeta := metav1.TypeMeta{Kind: "ConfigMap", APIVersion: "v1"}
	var keyValue = make(map[string]string)

	if cm.Data != nil {
		keyValueBytes := keyValueFromData(cm.Data)
		for k, v := range keyValueBytes {
			keyValue[k] = string(v)
		}
	}

	configmap := &apiv1.ConfigMap{Data: keyValue, ObjectMeta: object, TypeMeta: typeMeta}
	return configmap
}

// generateSecret generates a Secret starting from configurations stored in `Secret` struct
func generateSecret(secret secret) *apiv1.Secret {

	var secretType apiv1.SecretType
	typeMeta := metav1.TypeMeta{Kind: "Secret", APIVersion: "v1"}
	var keyValue = make(map[string][]byte)
	when := secret.When
	object := metav1.ObjectMeta{
		Name: secret.Name,
		Annotations: map[string]string{
			"mia-platform.eu/deploy": when,
		},
	}

	switch {
	case secret.Data != nil:
		secretType = apiv1.SecretTypeOpaque
		keyValue = keyValueFromData(secret.Data)
	case secret.Docker != docker{}:
		secretType = apiv1.SecretTypeDockerConfigJson
		keyValue[".dockerconfigjson"] = genDockercfg(secret.Docker)
	case !cmp.Equal(secret.TLS, tls{}):
		secretType = apiv1.SecretTypeTLS
		keyValue["tls.key"] = readTLSData(secret.TLS.Key)
		keyValue["tls.crt"] = readTLSData(secret.TLS.Cert)
	default:
		panic("unknown secret type")
	}

	return &apiv1.Secret{
		Data:       keyValue,
		ObjectMeta: object,
		TypeMeta:   typeMeta,
		Type:       secretType,
	}
}
