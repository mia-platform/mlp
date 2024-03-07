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

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/mia-platform/mlp/internal/cli"
	"github.com/spf13/cobra"
)

var options = cli.New()

func main() {
	rootCmd := &cobra.Command{
		Short: "mlp CLI",
		Long:  "Deploy Mia-Platform applications on Kubernetes.",
		Use:   "mlp",

		SilenceErrors: true,
		SilenceUsage:  true,
		Example: heredoc.Doc(`
				$ mlp interpolate
				$ mlp deploy -f ./folder
				$ mlp generate --env-prefix DEV_`),
	}

	versionOutput := versionFormat(cli.Version, cli.BuildDate)
	// Version subcommand
	rootCmd.AddCommand(&cobra.Command{
		Use:   "version",
		Short: "Show mlp version",
		Long:  "Show mlp version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(versionOutput)
		},
	})

	cli.AddGlobalFlags(rootCmd, options)
	cli.InterpolateSubcommand(rootCmd)
	cli.DeploySubcommand(rootCmd, options)
	cli.GenerateSubcommand(rootCmd)
	cli.KustomizeSubcommand(rootCmd)
	cli.HydrateSubcommand(rootCmd)

	expandedArgs := []string{}
	if len(os.Args) > 0 {
		expandedArgs = os.Args[1:]
	}
	rootCmd.SetArgs(expandedArgs)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// versionFormat return the version string nicely formatted
func versionFormat(version, buildDate string) string {
	if buildDate != "" {
		version = fmt.Sprintf("%s (%s)", version, buildDate)
	}

	version = fmt.Sprintf("mlp version: %s", version)
	// don't return GoVersion during a test run for consistent test output
	if flag.Lookup("test.v") != nil {
		return version
	}

	return fmt.Sprintf("%s, Go Version: %s", version, runtime.Version())
}
