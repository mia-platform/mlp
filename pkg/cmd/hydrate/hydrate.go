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

package hydrate

import (
	"cmp"
	"context"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	cmdUsage = "hydrate [DIR]..."
	cmdShort = "Generate 'kustomization.yaml' files with content of DIRs"
	cmdLong  = `Generate 'kustomization.yaml' files with content of DIRs.

	The command will create a new 'kustomization.yaml' file if does not already exists
	in the target folders and then will add all the file called '*.patch.yaml' or
	'*.patch.yml' as patches and all the rest '*.yaml' or '*.yml' file as resources.
	`
	cmdExamples = `# hydrate only one folder
	mlp hydrate configuration

	# hydrate multiple folders
	mlp hydrate configuration overlays/production

	# hydrate current folder
	mlp hydrate
	`
)

// Flags contains all the flags for the `hydrate` command. They will be converted to Options
// that contains all runtime options for the command.
type Flags struct{}

// Options have the data required to perform the hydrate operation
type Options struct {
	paths []string
	fSys  filesys.FileSystem
}

// NewCommand return the command for generating kustomization files in target folders and populating the resource
// property with the contents alredy present
func NewCommand() *cobra.Command {
	flags := &Flags{}
	cmd := &cobra.Command{
		Use:     cmdUsage,
		Short:   heredoc.Doc(cmdShort),
		Long:    heredoc.Doc(cmdLong),
		Example: heredoc.Doc(cmdExamples),

		Run: func(cmd *cobra.Command, args []string) {
			o, err := flags.ToOptions(args, filesys.MakeFsOnDisk())
			cobra.CheckErr(err)
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	return cmd
}

// ToOptions transform the command flags in command runtime arguments
func (*Flags) ToOptions(args []string, fSys filesys.FileSystem) (*Options, error) {
	var paths []string
	switch len(args) {
	case 0:
		paths = append(paths, filesys.SelfDir)
	default:
		paths = args
	}

	return &Options{
		paths: paths,
		fSys:  fSys,
	}, nil
}

// Run execute the hydrate command
func (o *Options) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)

	logger.V(5).Info("hydrating files", "paths", strings.Join(o.paths, ", "))
	for _, path := range o.paths {
		if err := o.hydrateKustomize(ctx, path); err != nil {
			return err
		}
	}

	return nil
}

// hydrateKustomize will read the folder at path and insert the files as resources or patches based on a regex
func (o *Options) hydrateKustomize(ctx context.Context, path string) error {
	logger := logr.FromContextOrDiscard(ctx)

	logger.V(5).Info("hydrating", "path", path)
	files, err := o.fSys.ReadDir(path)
	if err != nil {
		return err
	}

	logger.V(8).Info("files found", "paths", strings.Join(files, ", "))
	var resources []string
	var patches []string
	regex := regexp.MustCompile(`(^|\.)patch\.ya?ml$`)
	yamlExtensions := []string{".yaml", ".yml"}
	for _, file := range files {
		normalizedName := strings.ToLower(file)
		extension := filepath.Ext(normalizedName)
		if !slices.Contains(yamlExtensions, extension) {
			continue
		}

		switch regex.MatchString(normalizedName) {
		case true:
			logger.V(10).Info("patch found", "path", file)
			patches = append(patches, file)
		case false:
			logger.V(10).Info("resource found", "path", file)
			resources = append(resources, file)
		}
	}

	slices.SortStableFunc(resources, cmp.Compare)
	slices.SortStableFunc(patches, cmp.Compare)
	return updateKustomize(ctx, o.fSys, path, resources, patches)
}

// updateKustomize will read the kustomization file at path and will add resources and patches if not already
// present in the file
func updateKustomize(ctx context.Context, fSys filesys.FileSystem, path string, resources, patches []string) error {
	logger := logr.FromContextOrDiscard(ctx)

	kf, err := newKustomizationFile(fSys, path)
	if err != nil {
		return err
	}

	logger.V(3).Info("reading kustomization file", "path", path)
	k, err := kf.read()
	if err != nil {
		return err
	}

	for _, resource := range resources {
		if kf.GetPath() == filepath.Join(path, resource) || slices.Contains(k.Resources, resource) {
			continue
		}
		logger.V(8).Info("adding resource", "path", resource)
		k.Resources = append(k.Resources, resource)
	}

	for _, patch := range patches {
		p := types.Patch{
			Path: patch,
		}

		found := false
		for _, pp := range k.Patches {
			if pp.Equals(p) {
				found = true
				break
			}
		}
		if !found {
			logger.V(8).Info("adding patch", "path", patch)
			k.Patches = append(k.Patches, p)
		}
	}

	logger.V(5).Info("saving kustomization file", "path", path)
	return kf.write(k)
}
