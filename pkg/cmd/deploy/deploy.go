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

package deploy

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/go-logr/logr"
	"github.com/mia-platform/jpl/pkg/client"
	"github.com/mia-platform/jpl/pkg/flowcontrol"
	"github.com/mia-platform/jpl/pkg/generator"
	"github.com/mia-platform/jpl/pkg/resourcereader"
	"github.com/mia-platform/jpl/pkg/util"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	corev1 "k8s.io/client-go/applyconfigurations/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/utils/clock"
)

const (
	cmdUsage = "deploy"
	cmdShort = "Deploy kubernetes resources generated by Mia-Platform"
	cmdLong  = `Deploy kubernetes resources generated by Mia-Platform.

	Additionally to traditional 'kubectl apply', this command has additional
	capabilities, like keeping track of deployed resources for removing them
	when not present anymore, forcing deployment rollout when no changes
	to the manifest are present and generating annotations for mounted files.
	`

	inputPathsFlagName  = "filename"
	inputPathsShortName = "f"
	inputPathsFlagUsage = "the files and/or folders that contain the configurations to apply. Use '-' for reading from stdin"

	deployTypeFlagName     = "deploy-type"
	deployTypeDefaultValue = deployAll
	deployTypeFlagUsage    = "set the deployment mode (accepted values: deploy_all, smart_deploy)"

	forceDeployFlagName     = "force-deploy-when-no-semver"
	forceDeployDefaultValue = false
	forceDeployFlagUsage    = "force deployment rollout if their image tag doesn't follow semantic versioning"

	ensureNamespaceFlagName     = "ensure-namespace"
	ensureNamespaceDefaultValue = true
	ensureNamespaceFlagUsage    = "if false no control on the target namespace is done, if true and the namspace don't exists it will be created"

	dryRunFlagName     = "dry-run"
	dryRunDefaultValue = false
	dryRunFlagUsage    = "if true the resources will be sent to the cluster but not persisted"

	stdinToken    = "-"
	fieldManager  = "mlp"
	inventoryName = "eu.mia-platform.mlp"
)

// Flags contains all the flags for the `deploy` command. They will be converted to Options
// that contains all runtime options for the command.
type Flags struct {
	ConfigFlags     *genericclioptions.ConfigFlags
	inputPaths      []string
	deployType      string
	forceDeploy     bool
	ensureNamespace bool
	dryRun          bool
}

// Options have the data required to perform the deploy operation
type Options struct {
	inputPaths      []string
	deployType      string
	forceDeploy     bool
	ensureNamespace bool
	dryRun          bool

	clientFactory util.ClientFactory
	clock         clock.PassiveClock
	reader        io.Reader
	writer        io.Writer
}

// NewCommand return the command for deploying kubernetes resources against the target cluster
func NewCommand(configFlags *genericclioptions.ConfigFlags) *cobra.Command {
	flags := &Flags{
		ConfigFlags: configFlags,
	}

	cmd := &cobra.Command{
		Use:   cmdUsage,
		Short: heredoc.Doc(cmdShort),
		Long:  heredoc.Doc(cmdLong),

		Args: cobra.NoArgs,

		PreRun: func(cmd *cobra.Command, _ []string) {
			logger := logr.FromContextOrDiscard(cmd.Context())
			restClient, err := flags.ConfigFlags.ToRESTConfig()
			cobra.CheckErr(err)
			logger.V(10).Info("checking flow control APIs")
			enabled, err := flowcontrol.IsEnabled(cmd.Context(), restClient)
			cobra.CheckErr(err)
			qps := float32(100.0)
			burst := 500
			if enabled {
				qps = -1
				burst = -1
			}
			flags.ConfigFlags.WrapConfigFn = func(c *rest.Config) *rest.Config {
				c.QPS = qps
				c.Burst = burst
				return c
			}
			logger.V(5).Info("flow control APIs", "enabled", enabled)
		},
		Run: func(cmd *cobra.Command, _ []string) {
			o, err := flags.ToOptions(cmd.InOrStdin(), cmd.OutOrStderr())
			cobra.CheckErr(err)
			cobra.CheckErr(o.Validate())
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	flags.AddFlags(cmd.Flags())
	if err := cmd.RegisterFlagCompletionFunc(deployTypeFlagName, deployTypeFlagCompletionfunc); err != nil {
		panic(err)
	}

	return cmd
}

// AddFlags set the connection between Flags property to command line flags
func (f *Flags) AddFlags(flags *pflag.FlagSet) {
	if f.ConfigFlags != nil {
		f.ConfigFlags.AddFlags(flags)
	}

	flags.StringSliceVarP(&f.inputPaths, inputPathsFlagName, inputPathsShortName, nil, inputPathsFlagUsage)
	flags.StringVar(&f.deployType, deployTypeFlagName, deployTypeDefaultValue, deployTypeFlagUsage)
	flags.BoolVar(&f.forceDeploy, forceDeployFlagName, forceDeployDefaultValue, forceDeployFlagUsage)
	flags.BoolVar(&f.ensureNamespace, ensureNamespaceFlagName, ensureNamespaceDefaultValue, ensureNamespaceFlagUsage)
	flags.BoolVar(&f.dryRun, dryRunFlagName, dryRunDefaultValue, dryRunFlagUsage)
}

// ToOptions transform the command flags in command runtime arguments
func (f *Flags) ToOptions(reader io.Reader, writer io.Writer) (*Options, error) {
	if f.ConfigFlags == nil {
		return nil, fmt.Errorf("config flags are required")
	}

	return &Options{
		inputPaths:      f.inputPaths,
		deployType:      f.deployType,
		forceDeploy:     f.forceDeploy,
		ensureNamespace: f.ensureNamespace,

		clientFactory: util.NewFactory(f.ConfigFlags),
		reader:        reader,
		writer:        writer,
		clock:         clock.RealClock{},
	}, nil
}

func (o *Options) Validate() error {
	if len(o.inputPaths) == 0 {
		return fmt.Errorf("at least one path must be specified with %q flag", inputPathsFlagName)
	}

	if len(o.inputPaths) > 1 && slices.Contains(o.inputPaths, stdinToken) {
		return fmt.Errorf("cannot read from stdin and other paths together")
	}

	if !slices.Contains(validDeployTypeValues, o.deployType) {
		return fmt.Errorf("invalid deploy type value: %q", o.deployType)
	}

	return nil
}

// Run execute the interpolate command
func (o *Options) Run(ctx context.Context) error {
	namespace, _, err := o.clientFactory.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	inventory, err := NewInventory(o.clientFactory, inventoryName, namespace, fieldManager)
	if err != nil {
		return err
	}

	resources, err := o.readResources()
	if err != nil {
		return err
	}

	if err := o.ensuringNamespace(ctx, namespace); err != nil {
		return nil
	}

	deployIdentifier := map[string]string{
		"time": o.clock.Now().Format(time.RFC3339),
	}

	applyClient, err := client.NewBuilder().
		WithFactory(o.clientFactory).
		WithInventory(inventory).
		WithGenerators(generator.NewJobGenerator(jobGeneratorLabel, jobGeneratorValue)).
		WithMutator(
			NewDependenciesMutator(resources),
			NewDeployMutator(o.deployType, o.forceDeploy, checksumFromData(deployIdentifier)),
		).
		WithFilters(NewDeployOnceFilter()).
		Build()
	if err != nil {
		return err
	}
	opts := client.ApplierOptions{
		FieldManager: fieldManager,
		DryRun:       o.dryRun,
	}

	eventCh := applyClient.Run(ctx, resources, opts)

loop:
	for {
		select {
		case event, open := <-eventCh:
			if !open {
				break loop
			}

			fmt.Fprintln(o.writer, event.String())
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

func deployTypeFlagCompletionfunc(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return validDeployTypeValues, cobra.ShellCompDirectiveDefault
}

func (o *Options) readResources() ([]*unstructured.Unstructured, error) {
	readerBuilder := resourcereader.NewResourceReaderBuilder(o.clientFactory)
	var accumulatedResources []*unstructured.Unstructured

	for _, path := range o.inputPaths {
		reader, err := readerBuilder.ResourceReader(o.reader, path)
		if err != nil {
			return nil, err
		}

		resources, err := reader.Read()
		if err != nil {
			return nil, err
		}

		accumulatedResources = append(accumulatedResources, resources...)
	}

	return accumulatedResources, nil
}

func (o *Options) ensuringNamespace(ctx context.Context, namespace string) error {
	if !o.ensureNamespace {
		return nil
	}

	opts := metav1.ApplyOptions{
		Force:        true,
		FieldManager: fieldManager,
	}

	if o.dryRun {
		opts.DryRun = []string{metav1.DryRunAll}
	}
	clientSet, err := o.clientFactory.KubernetesClientSet()
	if err != nil {
		return err
	}

	namespaceApply := corev1.Namespace(namespace)
	_, err = clientSet.CoreV1().Namespaces().Apply(ctx, namespaceApply, opts)
	return err
}
