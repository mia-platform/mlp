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

package kustomize

import (
	"context"
	"errors"
	"io"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	cmdUsage = "kustomize DIR"
	cmdShort = "Build a kustomization target from a directory"
	cmdLong  = `Build a set of KRM resources using a 'kustomization.yaml' file.
	The DIR argument must be a path to a directory containing a
	'kustomization.yaml' file.
	If DIR is omitted, '.' is assumed.
	`
	cmdExamples = `# Build the current working directory
	mlp kustomize

	# Build a specific path
	mlp kustomize /home/config/project

	# Save output to a file
	mlp kustomize --output /home/config/build-results.yaml
	`

	outputFlagName          = "output"
	outputFlagShort         = "o"
	outputFlagUsage         = "If specified, write output to the file at this path"
	outputIsADirectoryError = "output path is a directory instead of a file"
)

// Flags contains all the flags for the `kustomize` command. They will be converted to Options
// that contains all runtime options for the command.
type Flags struct {
	outputPath string
}

// Options have the data required to perform the kustomize operation
type Options struct {
	inputPath  string
	outputPath string
	fSys       filesys.FileSystem
	writer     io.Writer
}

// NewCommand return the command for build a kustomization target from a directory
func NewCommand() *cobra.Command {
	flags := &Flags{}
	cmd := &cobra.Command{
		Use:     cmdUsage,
		Short:   heredoc.Doc(cmdShort),
		Long:    heredoc.Doc(cmdLong),
		Example: heredoc.Doc(cmdExamples),

		Args: cobra.RangeArgs(0, 1),

		Run: func(cmd *cobra.Command, args []string) {
			o, err := flags.ToOptions(args, filesys.MakeFsOnDisk(), cmd.OutOrStderr())
			cobra.CheckErr(err)
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	flags.AddFlags(cmd.Flags())

	return cmd
}

// AddFlags set the connection between Flags property to command line flags
func (f *Flags) AddFlags(set *pflag.FlagSet) {
	set.StringVarP(&f.outputPath, outputFlagName, outputFlagShort, "", outputFlagUsage)
}

// ToOptions transform the command flags in command runtime arguments
func (f *Flags) ToOptions(args []string, fSys filesys.FileSystem, writer io.Writer) (*Options, error) {
	var inputPath string
	switch len(args) {
	case 0:
		inputPath = filesys.SelfDir
	default:
		inputPath = args[0]
	}

	if len(f.outputPath) > 0 && fSys.IsDir(f.outputPath) {
		return nil, errors.New(outputIsADirectoryError)
	}

	return &Options{
		inputPath:  inputPath,
		outputPath: f.outputPath,
		fSys:       fSys,
		writer:     writer,
	}, nil
}

// Run execute the kustomize command
func (o *Options) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	logger.V(5).Info("reading kustomize files", "path", o.inputPath)
	kustomizer := krusty.MakeKustomizer(krusty.MakeDefaultOptions())
	resourceMap, err := kustomizer.Run(o.fSys, o.inputPath)
	if err != nil {
		return err
	}

	yaml, err := resourceMap.AsYaml()
	if err != nil {
		return err
	}

	if len(o.outputPath) > 0 {
		logger.V(5).Info("writing accumulated data", "path", o.outputPath)
		return o.fSys.WriteFile(o.outputPath, yaml)
	}

	_, err = o.writer.Write(yaml)
	return err
}
