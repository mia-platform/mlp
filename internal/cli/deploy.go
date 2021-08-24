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

package cli

import (
	"git.tools.mia-platform.eu/platform/devops/deploy/internal/utils"
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/deploy"
	"github.com/spf13/cobra"
)

// DeploySubcommand add deploy subcommand to the main command
func DeploySubcommand(cmd *cobra.Command, options *utils.Options) {
	var inputPaths []string
	var deployConfig utils.DeployConfig

	deployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "deploy Mia-Platform created resource on K8S",
		Long:  "",
		Run: func(cmd *cobra.Command, args []string) {
			deploy.Run(inputPaths, deployConfig, options)
		},
	}

	deployCmd.Flags().StringSliceVarP(&inputPaths, "filename", "f", []string{}, "file and/or folder paths containing data to interpolate")
	deployCmd.Flags().StringVar(&deployConfig.DeployType, "deploy-type", "deploy_all", "Set the deployment type (accepted values: deploy_all, smart_deploy)")
	deployCmd.Flags().BoolVar(&deployConfig.ForceDeployWhenNoSemver, "force-deploy-when-no-semver", false, "Set whether deployment for services not following semantic versioning should be deployed forcibly (useful when using --deploy-type=smart_deploy")
	deployCmd.Flags().BoolVar(&deployConfig.EnsureNamespace, "ensure-namespace", true, "Set if the namespace existence should be ensured. By default it is set to true so that the namespace existence is checked and, if it not exists, created. If set to false, it throws if namespace not already exists")

	cmd.AddCommand(deployCmd)
}
