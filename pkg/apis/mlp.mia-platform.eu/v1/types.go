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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const (
	GroupName = "mlp.mia-platform.eu"

	DataFromFile    = "file"
	DataFromLiteral = "literal"
)

var SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}

type GenerateConfiguration struct {
	metav1.TypeMeta `json:",inline" yaml:",inline"`

	Secrets []SecretSpec `json:"secrets,omitempty" yaml:"secrets,omitempty"`

	//nolint:tagliatelle
	ConfigMaps []ConfigMapSpec `json:"config-maps,omitempty" yaml:"config-maps,omitempty"`
}

// SecretSpec contains secret configurations
type SecretSpec struct {
	Name   string        `json:"name" yaml:"name"`
	When   string        `json:"when" yaml:"when"`
	TLS    *TLS          `json:"tls" yaml:"tls"`
	Docker *DockerConfig `json:"docker" yaml:"docker"`
	Data   []Data        `json:"data" yaml:"data"`
}

type ConfigMapSpec struct {
	Name string `json:"name" yaml:"name"`
	Data []Data `json:"data" yaml:"data"`
}

type TLS struct {
	Cert *TLSData `json:"cert" yaml:"cert"`
	Key  *TLSData `json:"key" yaml:"key"`
}

type TLSData struct {
	From  string `json:"from" yaml:"from"`
	File  string `json:"file" yaml:"file"`
	Value string `json:"value" yaml:"value"`
}

type DockerConfig struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
	Email    string `json:"email" yaml:"email"`
	Server   string `json:"server" yaml:"server"`
}

type Data struct {
	From  string `json:"from" yaml:"from"`
	File  string `json:"file" yaml:"file"`
	Key   string `json:"key" yaml:"key"`
	Value string `json:"value" yaml:"value"`
}
