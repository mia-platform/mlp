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
	deployCmd := &cobra.Command{
		Use:   "deploy",
		Short: "deploy Mia-Platform created resource on K8S",
		Long:  "",
		Run: func(cmd *cobra.Command, args []string) {
			deploy.Run(inputPaths, options)
		},
	}

	deployCmd.Flags().StringSliceVarP(&inputPaths, "filename", "f", []string{}, "file and/or folder paths containing data to interpolate")
	cmd.AddCommand(deployCmd)
}
