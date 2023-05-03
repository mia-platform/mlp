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
	"errors"
	"fmt"
	"os"

	"github.com/mia-platform/mlp/pkg/kustomize"
	"github.com/spf13/cobra"
	"k8s.io/kubectl/pkg/util/i18n"
	"k8s.io/kubectl/pkg/util/templates"
	"sigs.k8s.io/kustomize/kustomize/v4/commands/build"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// KustomizeSubcommand runs kustomize
func KustomizeSubcommand(cmd *cobra.Command) {
	h := build.MakeHelp("mlp", "kustomize")
	KustomizeCmd := build.NewCmdBuild(
		filesys.MakeFsOnDisk(),
		&build.Help{
			Use:     h.Use,
			Short:   i18n.T(h.Short),
			Long:    templates.LongDesc(i18n.T(h.Long)),
			Example: templates.Examples(i18n.T(h.Example)),
		},
		os.Stdout)
	cmd.AddCommand(KustomizeCmd)
}

// HydrateSubcommand add resources and patches to kustomization.yaml
func HydrateSubcommand(cmd *cobra.Command) {
	hydrateCmd := &cobra.Command{
		Use:     "hydrate [PATH]...",
		Short:   "Adds passed resource and patches to kustomization.yaml",
		Long:    "Adds passed resource and patches to kustomization.yaml. The PATH argument must be a path to a directory containing 'kustomization.yaml'",
		Example: "$ mlp hydrate configuration/ overlays/production/",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("at least one directory is required")
			}
			for _, d := range args {
				fileInfo, err := os.Stat(d)
				if err != nil {
					return err
				}
				if !fileInfo.IsDir() {
					return fmt.Errorf("%s is not a directory", d)
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, paths []string) error {
			return kustomize.HydrateRun(paths)
		},
	}
	cmd.AddCommand(hydrateCmd)
}
