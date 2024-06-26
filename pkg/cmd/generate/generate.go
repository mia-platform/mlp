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
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"github.com/MakeNowJust/heredoc/v2"
	v1 "github.com/mia-platform/mlp/pkg/apis/mlp.mia-platform.eu/v1"
	"github.com/mia-platform/mlp/pkg/cmd/interpolate"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kustomize/kyaml/filesys"
	"sigs.k8s.io/yaml"
)

const (
	cmdUsage = "generate"
	cmdShort = "Generate ConfigMap and Secret manifests"
	cmdLong  = `Generate ConfigMap and Secret Kubernetes manifest files from one or more
	configuration files.

	The configuration files will be interpolated with the same logic of the
	interpolate command.
	`

	configFilesFlagName  = "config-file"
	configFilesShortName = "c"
	configFilesFlagUsage = "config file that contains resources definitions"

	prefixesFlagName  = "env-prefix"
	prefixesFlagShort = "e"
	prefixesFlagUsage = "prefixes to add when looking for ENV variables"

	outputFlagName  = "out"
	outputFlagShort = "o"
	outputFlagUsage = "output directory where interpolated files are saved"
)

var (
	validExtensions = []string{".yaml", ".yml"}
)

// Flags contains all the flags for the `generate` command. They will be converted to Options
// that contains all runtime options for the command.
type Flags struct {
	configFiles []string
	prefixes    []string
	outputPath  string
}

// Options have the data required to perform the generate operation
type Options struct {
	configFiles []string
	prefixes    []string
	outputPath  string
	fSys        filesys.FileSystem
}

// NewCommand return the command for generating ConfigMap and Secret resources from a configuration file
func NewCommand() *cobra.Command {
	flags := &Flags{}
	cmd := &cobra.Command{
		Use:   cmdUsage,
		Short: heredoc.Doc(cmdShort),
		Long:  heredoc.Doc(cmdLong),

		Args: cobra.NoArgs,

		Run: func(cmd *cobra.Command, _ []string) {
			o, err := flags.ToOptions(filesys.MakeFsOnDisk())
			cobra.CheckErr(err)
			cobra.CheckErr(o.Validate())
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	flags.AddFlags(cmd.Flags())

	return cmd
}

// AddFlags set the connection between Flags property to command line flags
func (f *Flags) AddFlags(flags *pflag.FlagSet) {
	flags.StringSliceVarP(&f.configFiles, configFilesFlagName, configFilesShortName, nil, configFilesFlagUsage)
	if err := cobra.MarkFlagFilename(flags, configFilesFlagName, validExtensions...); err != nil {
		panic(err)
	}

	flags.StringSliceVarP(&f.prefixes, prefixesFlagName, prefixesFlagShort, nil, prefixesFlagUsage)
	flags.StringVarP(&f.outputPath, outputFlagName, outputFlagShort, "interpolated-files", outputFlagUsage)
	if err := cobra.MarkFlagDirname(flags, outputFlagName); err != nil {
		panic(err)
	}
}

// ToOptions transform the command flags in command runtime arguments
func (f *Flags) ToOptions(fSys filesys.FileSystem) (*Options, error) {
	return &Options{
		configFiles: f.configFiles,
		prefixes:    f.prefixes,
		outputPath:  f.outputPath,
		fSys:        fSys,
	}, nil
}

func (o *Options) Validate() error {
	if len(o.configFiles) == 0 {
		return fmt.Errorf("at least one config file must be specified")
	}

	return nil
}

// Run execute the generate command
func (o *Options) Run(context.Context) error {
	if err := o.fSys.MkdirAll(o.outputPath); err != nil {
		return err
	}

	pathsToInterpolate := o.filterYAMLFiles()
	for _, path := range pathsToInterpolate {
		configuration, err := o.readConfiguration(path)
		if err != nil {
			return err
		}

		err = o.generateResources(configuration)
		if err != nil {
			return err
		}
	}

	return nil
}

func (o *Options) filterYAMLFiles() []string {
	filteredPaths := make([]string, 0)
	for _, path := range o.configFiles {
		if o.fSys.IsDir(path) || !slices.Contains(validExtensions, filepath.Ext(path)) {
			continue
		}
		filteredPaths = append(filteredPaths, path)
	}

	return filteredPaths
}

func (o *Options) readConfiguration(path string) (*v1.GenerateConfiguration, error) {
	data, err := o.fSys.ReadFile(path)
	if err != nil {
		return nil, err
	}

	interpolatedData, err := interpolate.Interpolate(data, o.prefixes, filepath.Base(path))
	if err != nil {
		return nil, err
	}

	configuration := new(v1.GenerateConfiguration)
	err = yaml.Unmarshal(interpolatedData, configuration)
	println(configuration)
	return configuration, err
}

func (o *Options) generateResources(config *v1.GenerateConfiguration) error {
	resources := make(map[string]runtime.Object, len(config.Secrets)+len(config.ConfigMaps))
	for _, obj := range config.ConfigMaps {
		cm, err := o.configMapFromConfig(obj)
		if err != nil {
			return err
		}

		name := fmt.Sprintf("%s.configmap.yaml", obj.Name)
		resources[name] = cm
	}

	for _, obj := range config.Secrets {
		sec, err := o.secretsFromConfig(obj)
		if err != nil {
			return err
		}

		name := fmt.Sprintf("%s.secret.yaml", obj.Name)
		resources[name] = sec
	}

	for name, obj := range resources {
		data, err := yaml.Marshal(obj)
		if err != nil {
			return err
		}

		if err := o.fSys.WriteFile(filepath.Join(o.outputPath, name), data); err != nil {
			return err
		}
	}

	return nil
}

func (o *Options) configMapFromConfig(spec v1.ConfigMapSpec) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: spec.Name,
		},
	}
	configMap.Name = spec.Name
	configMap.Data = map[string]string{}
	configMap.BinaryData = map[string][]byte{}

	for _, data := range spec.Data {
		switch data.From {
		case v1.DataFromLiteral:
			configMap.Data[data.Key] = data.Value
		case v1.DataFromFile:
			content, err := o.fSys.ReadFile(data.File)
			if err != nil {
				return nil, err
			}
			key := filepath.Base(data.File)
			switch utf8.Valid(content) {
			case true:
				configMap.Data[key] = string(content)
			default:
				configMap.BinaryData[key] = content
			}
		}
	}

	return configMap, nil
}

func (o *Options) secretsFromConfig(spec v1.SecretSpec) (*corev1.Secret, error) {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: corev1.SchemeGroupVersion.String(),
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: spec.Name,
			Annotations: map[string]string{
				"mia-platform.eu/deploy": spec.When,
			},
		},
		Data: map[string][]byte{},
	}

	switch {
	case spec.Data != nil:
		secret.Type = corev1.SecretTypeOpaque
		for _, data := range spec.Data {
			switch data.From {
			case v1.DataFromLiteral:
				secret.Data[data.Key] = []byte(data.Value)
			case v1.DataFromFile:
				content, err := o.fSys.ReadFile(data.File)
				if err != nil {
					return nil, err
				}
				secret.Data[filepath.Base(data.File)] = content
			}
		}
	case spec.Docker != nil:
		secret.Type = corev1.SecretTypeDockerConfigJson
		data, err := parseDocker(spec.Docker)
		if err != nil {
			return nil, err
		}
		secret.Data[corev1.DockerConfigJsonKey] = data
	case spec.TLS != nil:
		secret.Type = corev1.SecretTypeTLS
		certData, certKey, err := o.parseTLS(spec.TLS)
		if err != nil {
			return nil, err
		}
		secret.Data[corev1.TLSCertKey] = certData
		secret.Data[corev1.TLSPrivateKeyKey] = certKey
	}

	return secret, nil
}

func parseDocker(dockerConfig *v1.DockerConfig) ([]byte, error) {
	dockerConfigAuth := dockerConfigEntry{
		Username: dockerConfig.Username,
		Password: dockerConfig.Password,
		Email:    dockerConfig.Email,
		Auth:     encodeDockerConfigFieldAuth(dockerConfig.Username, dockerConfig.Password),
	}
	dockerConfigJSON := dockerConfigJSON{
		Auths: map[string]dockerConfigEntry{dockerConfig.Server: dockerConfigAuth},
	}

	return json.Marshal(dockerConfigJSON)
}

// encodeDockerConfigFieldAuth returns base64 encoding of the username and password string
func encodeDockerConfigFieldAuth(username, password string) string {
	fieldValue := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(fieldValue))
}

func (o *Options) parseTLS(tlsConfig *v1.TLS) ([]byte, []byte, error) {
	tlsCert, tlsKey, err := o.readTLSData(tlsConfig)
	if err != nil {
		return nil, nil, err
	}

	if _, err := tls.X509KeyPair(tlsCert, tlsKey); err != nil {
		return nil, nil, err
	}

	return tlsCert, tlsKey, nil
}

func (o *Options) readTLSData(data *v1.TLS) ([]byte, []byte, error) {
	tls := make([][]byte, 2)
	var err error
loop:
	for idx, tlsData := range []*v1.TLSData{data.Cert, data.Key} {
		if tlsData == nil {
			continue
		}

		switch tlsData.From {
		case v1.DataFromLiteral:
			tls[idx] = []byte(tlsData.Value)
		case v1.DataFromFile:
			content, readErr := o.fSys.ReadFile(tlsData.File)
			tls[idx] = content
			err = readErr
		default:
			err = fmt.Errorf("unknown data source: %s", tlsData.From)
			break loop
		}
	}

	return tls[0], tls[1], err
}
