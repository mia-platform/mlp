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

// -------------- configmaps and secrets scheme ----------------

// cfgCmapsSecrets top level configuration file keys
type generateConfiguration struct {
	Secret    []secret    `json:"secrets,omitempty"`
	ConfigMap []configMap `json:"config-maps,omitempty"`
}

// secret contains secrets configurations
type secret struct {
	Name   string `json:"name"`
	When   string `json:"when"`
	TLS    tls    `json:"tls"`
	Docker docker `json:"docker"`
	Data   data   `json:"data"`
}

// configMap contains configmap configurations
type configMap struct {
	Name string `json:"name"`
	Data data   `json:"data"`
}

// tls contains TlS secret data
type tls struct {
	Cert tlsData `json:"cert"`
	Key  tlsData `json:"key"`
}

// tlsData contains the cert or key info of a TLS secret
type tlsData struct {
	From  string `json:"from"`
	File  string `json:"file"`
	Value string `json:"value"`
}

// docker contains Docker Config data
type docker struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Server   string `json:"server"`
}

// data describes how to create Opaque secrets and ConfigMaps
type data []struct {
	From  string `json:"from"`
	File  string `json:"file"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// -------------- dockerconfigjson scheme ----------------

// dockerCfg captures the structure of file `dockerconfig.json`
type dockerCfg struct {
	Auths dockerConfig `json:"auths"`
}

// dockerConfig represents the list of registries in the `dockerconfig.json`
type dockerConfig map[string]dockerConfigEntry

// dockerConfigEntry caputures the structure of a single `docker registry`
type dockerConfigEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
	Auth     string `json:"auth"`
}
