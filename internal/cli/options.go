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

package cli

import (
	"os"

	"github.com/mia-platform/mlp/internal/utils"
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// New create a new options struct
func New() *utils.Options {
	options := &utils.Options{
		Kubeconfig: os.Getenv("KUBECONFIG"),
	}

	// bind to kubernetes config flags
	options.Config = &genericclioptions.ConfigFlags{
		CAFile:       &options.CertificateAuthority,
		CertFile:     &options.ClientCertificate,
		KeyFile:      &options.ClientKey,
		ClusterName:  &options.Cluster,
		Context:      &options.Context,
		KubeConfig:   &options.Kubeconfig,
		Insecure:     &options.InsecureSkipTLSVerify,
		Namespace:    &options.Namespace,
		APIServer:    &options.Server,
		BearerToken:  &options.Token,
		AuthInfoName: &options.User,
	}

	return options
}

// AddGlobalFlags add to the cobra command all the global flags
func AddGlobalFlags(cmd *cobra.Command, options *utils.Options) {
	flags := cmd.PersistentFlags()
	flags.StringVarP(&options.CertificateAuthority, "certificate-authority", "", "", "Path to a cert file for the certificate authority")
	flags.StringVarP(&options.ClientCertificate, "client-certificate", "", "", "Path to a client certificate file for TLS")
	flags.StringVarP(&options.ClientKey, "client-key", "", "", "Path to a client key file for TLS")
	flags.StringVarP(&options.Cluster, "cluster", "", "", "The name of the kubeconfig cluster to use")
	flags.StringVarP(&options.Context, "context", "", "", "The name of the kubeconfig context to use")
	flags.StringVarP(&options.Kubeconfig, "kubeconfig", "", "", "Path to the kubeconfig file to use for CLI requests")
	flags.StringVarP(&options.Namespace, "namespace", "n", "", "If present, the namespace scope for this CLI request")
	flags.StringVarP(&options.Server, "server", "s", "", "The address and port of the Kubernetes API server")
	flags.StringVarP(&options.Token, "token", "", "", "Bearer token for authentication to the API server")
	flags.StringVarP(&options.User, "user", "", "", "The name of the kubeconfig user to use")
	flags.BoolVarP(&options.InsecureSkipTLSVerify, "insecure-skip-tls-verify", "", false, "If true, the server's certificate will not be checked for validity. This will make your HTTPS connections insecure")
}
