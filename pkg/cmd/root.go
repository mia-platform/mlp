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

package cmd

import (
	"context"
	"fmt"
	"log"
	"runtime"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	"github.com/mia-platform/mlp/pkg/cmd/deploy"
	"github.com/mia-platform/mlp/pkg/cmd/generate"
	"github.com/mia-platform/mlp/pkg/cmd/hydrate"
	"github.com/mia-platform/mlp/pkg/cmd/interpolate"
	"github.com/mia-platform/mlp/pkg/cmd/kustomize"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var (
	// Version is dynamically set by the ci or overridden by the Makefile.
	Version = "DEV"
	// BuildDate is dynamically set at build time by the cli or overridden in the Makefile.
	BuildDate = "" // YYYY-MM-DD
)

const (
	cmdShort = "mlp can deploy a Mia-Platform application on a Kubernetes cluster"
	cmdLong  = `mlp can deploy a Mia-Platform application on a Kubernetes cluster.

	Handle resource files generated by Mia-Platform Console for correct deployment
	on Kubernetes. It provides additional capabilities on top of traditional
	"kubectl apply" like resource inventory, resource generation and force redeploy.`
	cmdExamples = `$ mlp interpolate
		$ mlp deploy -f ./folder
		$ mlp generate --env-prefix DEV_`

	versionCmdShort = "Show mlp version"
	versionCmdLong  = "Show mlp version"

	verboseFlagName      = "verbose"
	verboseFlagShortName = "v"
	verboseUsage         = "setting logging verbosity; use number between 0 and 10"
)

type Flags struct {
	verbosity int
}

func NewRootCommand() *cobra.Command {
	flags := &Flags{}
	cmd := &cobra.Command{
		Use: "mlp",

		Short:   heredoc.Doc(cmdShort),
		Long:    heredoc.Doc(cmdLong),
		Example: heredoc.Doc(cmdExamples),

		SilenceErrors: true,
		Version:       versionString(),

		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		PersistentPreRun: func(*cobra.Command, []string) {
			stdr.SetVerbosity(flags.verbosity)
		},
	}

	flags.AddFlags(cmd.PersistentFlags())

	logger := stdr.New(log.Default())
	cmd.SetContext(logr.NewContext(context.Background(), logger))

	cmd.AddCommand(
		deploy.NewCommand(genericclioptions.NewConfigFlags(true)),
		generate.NewCommand(),
		hydrate.NewCommand(),
		interpolate.NewCommand(),
		kustomize.NewCommand(),
		versionCommand(),
	)

	return cmd
}

// AddFlags set the connection between Flags property to command line flags
func (f *Flags) AddFlags(flags *pflag.FlagSet) {
	flags.IntVarP(&f.verbosity, verboseFlagName, verboseFlagShortName, f.verbosity, verboseUsage)
}

// versionCommand return the command for printing the version string, like --version flag
func versionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use: "version",

		Short: heredoc.Doc(versionCmdShort),
		Long:  heredoc.Doc(versionCmdLong),

		SilenceErrors:     true,
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		Run: func(cobra *cobra.Command, _ []string) {
			cobra.Println(versionString())
		},
	}

	return cmd
}

// versionString format a complete version string to output to the user
func versionString() string {
	version := Version

	if BuildDate != "" {
		version = fmt.Sprintf("%s (%s)", version, BuildDate)
	}

	return fmt.Sprintf("%s, Go Version: %s", version, runtime.Version())
}
