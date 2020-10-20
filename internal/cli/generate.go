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
	"git.tools.mia-platform.eu/platform/devops/deploy/pkg/generate"
	"github.com/spf13/cobra"
)

// GenerateSubcommand add generate subcommand to the main command
func GenerateSubcommand(cmd *cobra.Command, options *utils.Options) {
	var configPath []string
	var prefixes []string
	var outputPath string

	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate configmaps and secrets",
		Long:  `Generate configmaps and secrets described in YAML files passed as input`,
		Run: func(cmd *cobra.Command, args []string) {
			generate.Run(configPath, prefixes, outputPath)
		},
	}

	generateCmd.Flags().StringSliceVarP(&configPath, "config-file", "c", []string{}, "config file that contains all ConfigMaps and Secrets definitions")
	generateCmd.Flags().StringSliceVarP(&prefixes, "env-prefix", "e", []string{}, "Prefixes to add when looking for ENV variables")
	generateCmd.Flags().StringVarP(&outputPath, "out", "o", "./interpolated-files", "Output directory where interpolated fileNames are saved")
	cmd.AddCommand(generateCmd)
}
