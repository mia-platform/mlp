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
	"github.com/mia-platform/mlp/pkg/interpolate"
	"github.com/spf13/cobra"
)

// InterpolateSubcommand add interpolate subcommand to the main command
func InterpolateSubcommand(cmd *cobra.Command) {
	var prefixes []string
	var inputPaths []string
	var outputPath string

	interpolateCmd := &cobra.Command{
		Use:   "interpolate",
		Short: "Interpolate variables in file",
		Long:  "Interpolate the environment variables inside {{}} in file and substitutes them with the corresponding value",
		Run: func(cmd *cobra.Command, args []string) {
			interpolate.Run(prefixes, inputPaths, outputPath)
		},
	}

	interpolateCmd.Flags().StringSliceVarP(&prefixes, "env-prefix", "e", []string{}, "Prefixes to add when looking for ENV variables")
	interpolateCmd.Flags().StringSliceVarP(&inputPaths, "filename", "f", []string{}, "file/folder paths containing data to interpolate")
	interpolateCmd.Flags().StringVarP(&outputPath, "out", "o", "./interpolated-files", "Output directory where interpolated files are saved")
	cmd.AddCommand(interpolateCmd)
}
