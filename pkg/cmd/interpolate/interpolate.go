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

package interpolate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/MakeNowJust/heredoc/v2"
	"github.com/go-logr/logr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	cmdUsage = "interpolate"
	cmdShort = "Interpolate env variables in files"
	cmdLong  = `Interpolate the environment variables values delimited by '{{' and '}}' inside one or
	multiple files.
	If a path is a folder only the files directly inside will be interpolated.

	The results of the interpolation will be saved in the folder specified with
	the --out flag. By default the folder is named "interpolated-files".
	`

	cmdExamples = `# Interpolate a single file

	mlp interpolate --filename file.yaml

	# Interpolate from stdin

	mlp interpolate --filename -

	# Interpolate folder and a file

	mlp interpolate --filename a/folder --filename file.yaml

	# Interpolate a folder and save the resulting files in a custom folder

	mlp interpolate --filename a/folder --out result-folder/
	`

	prefixesFlagName  = "env-prefix"
	prefixesFlagShort = "e"
	prefixesFlagUsage = "prefixes to add when looking for ENV variables"

	inputFlagName  = "filename"
	inputFlagShort = "f"
	inputFlagUsage = "file or folder paths containing data to interpolate"

	allowMissingFilenamesFlagName    = "allow-missing-filenames"
	allowMissingFilenamesFlagUsage   = "set to allow missing input filenames"
	allowMissingFilenamesFlagDefault = false

	outputFlagName  = "out"
	outputFlagShort = "o"
	outputFlagUsage = "output directory where interpolated files are saved"

	stdinToken             = "-"
	outputFileNameForStdin = "output.yaml"

	envirnonmentRegex      = `[{]{2}([A-Z0-9_]+)[}]{2}`
	unqutedLeftDelim       = `{{`
	unqutedRightDelim      = `}}`
	doubleQoutedLeftDelim  = `"` + unqutedLeftDelim
	doubleQoutedRightDelim = unqutedRightDelim + `"`
	singleQoutedLeftDelim  = `'` + unqutedLeftDelim
	singleQoutedRightDelim = unqutedRightDelim + `'`
)

// Flags contains all the flags for the `interpolate` command. They will be converted to Options
// that contains all runtime options for the command.
type Flags struct {
	prefixes              []string
	inputPaths            []string
	allowMissingFilenames bool
	outputPath            string
}

// Options have the data required to perform the interpolate operation
type Options struct {
	prefixes              []string
	inputPaths            []string
	allowMissingFilenames bool
	outputPath            string
	fSys                  filesys.FileSystem
	reader                io.Reader
}

// NewCommand return the command for interpolating env variables on target files
func NewCommand() *cobra.Command {
	flags := &Flags{}
	cmd := &cobra.Command{
		Use:     cmdUsage,
		Short:   heredoc.Doc(cmdShort),
		Long:    heredoc.Doc(cmdLong),
		Example: heredoc.Doc(cmdExamples),

		Args: cobra.NoArgs,

		Run: func(cmd *cobra.Command, _ []string) {
			o, err := flags.ToOptions(cmd.InOrStdin(), filesys.MakeFsOnDisk())
			cobra.CheckErr(err)
			cobra.CheckErr(o.Validate())
			cobra.CheckErr(o.Run(cmd.Context()))
		},
	}

	flags.AddFlags(cmd.Flags())

	return cmd
}

// AddFlags set the connection between Flags property to command line flags
func (f *Flags) AddFlags(flags *pflag.FlagSet) {
	flags.StringSliceVarP(&f.prefixes, prefixesFlagName, prefixesFlagShort, nil, prefixesFlagUsage)
	flags.StringSliceVarP(&f.inputPaths, inputFlagName, inputFlagShort, nil, inputFlagUsage)
	flags.BoolVar(&f.allowMissingFilenames, allowMissingFilenamesFlagName, allowMissingFilenamesFlagDefault, allowMissingFilenamesFlagUsage)
	flags.StringVarP(&f.outputPath, outputFlagName, outputFlagShort, "interpolated-files", outputFlagUsage)
	if err := cobra.MarkFlagDirname(flags, outputFlagName); err != nil {
		panic(err)
	}
}

// ToOptions transform the command flags in command runtime arguments
func (f *Flags) ToOptions(reader io.Reader, fSys filesys.FileSystem) (*Options, error) {
	return &Options{
		inputPaths:            f.inputPaths,
		prefixes:              f.prefixes,
		allowMissingFilenames: f.allowMissingFilenames,
		outputPath:            f.outputPath,
		fSys:                  fSys,
		reader:                reader,
	}, nil
}

func (o *Options) Validate() error {
	if len(o.inputPaths) == 0 {
		return errors.New("at least one path must be specified")
	}

	if len(o.inputPaths) > 1 && slices.Contains(o.inputPaths, stdinToken) {
		return errors.New("cannot read from stdin and other paths together")
	}

	return nil
}

// Run execute the interpolate command
func (o *Options) Run(ctx context.Context) error {
	logger := logr.FromContextOrDiscard(ctx)
	if err := o.fSys.MkdirAll(o.outputPath); err != nil {
		return err
	}

	pathsToInterpolate, err := o.filesToInterpolate(ctx)
	if err != nil {
		return err
	}

	for _, path := range pathsToInterpolate {
		data, name, err := o.readFile(path)
		if err != nil {
			return err
		}

		logger.V(5).Info("intepolating file", "path", path)
		interpolatedData, err := Interpolate(data, o.prefixes)
		if err != nil {
			return err
		}

		logger.V(10).Info("saving interpolated file", "path", path)
		if err := o.fSys.WriteFile(filepath.Join(o.outputPath, name), interpolatedData); err != nil {
			return err
		}
	}

	return nil
}

func (o *Options) filesToInterpolate(ctx context.Context) ([]string, error) {
	logger := logr.FromContextOrDiscard(ctx)

	if o.inputPaths[0] == stdinToken {
		logger.V(10).Info("no paths provided, switch to stdin")
		return []string{stdinToken}, nil
	}

	logger.V(5).Info("accumulating files", "paths", strings.Join(o.inputPaths, ", "))
	yamlExtensions := []string{".yaml", ".yml"}
	var paths []string
	addOnlyYAMLFiles := func(path string) {
		logger.V(10).Info("considering file", "path", path)
		if slices.Contains(yamlExtensions, filepath.Ext(path)) {
			logger.V(10).Info("file has correct extension", "path", path)
			paths = append(paths, path)
		}
	}

	for _, path := range o.inputPaths {
		switch exists := o.fSys.Exists(path); {
		case !exists && o.allowMissingFilenames:
			logger.WithName("WARN").Info("can't read input file", "path", path)
			continue
		case !exists && !o.allowMissingFilenames:
			return nil, fmt.Errorf("no such file or directory: %s", path)
		}

		if !o.fSys.IsDir(path) {
			addOnlyYAMLFiles(path)
			continue
		}

		logger.V(10).Info("considering folder", "path", path)
		err := o.fSys.Walk(path, func(walkPath string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() && walkPath != path {
				logger.V(10).Info("ignore folder inside a folder", "path", walkPath)
				return fs.SkipDir
			}

			addOnlyYAMLFiles(walkPath)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	logger.V(5).Info("accumulated result", "paths", strings.Join(paths, ", "))
	return paths, nil
}

func (o *Options) readFile(path string) ([]byte, string, error) {
	if path == stdinToken {
		data, err := io.ReadAll(o.reader)
		return data, outputFileNameForStdin, err
	}

	data, err := o.fSys.ReadFile(path)
	return data, filepath.Base(path), err
}

// Interpolate will interpolate the data content with values from env values
func Interpolate(data []byte, envPrefixes []string) ([]byte, error) {
	for _, env := range envNamesToInterpolate(data) {
		parsedData, err := substituteEnv(string(data), env, envPrefixes)
		if err != nil {
			return nil, err
		}
		data = []byte(parsedData)
	}

	return data, nil
}

func envNamesToInterpolate(data []byte) []string {
	regex := regexp.MustCompile(envirnonmentRegex)
	envNames := make([]string, 0)
	for _, match := range regex.FindAllStringSubmatch(string(data), -1) {
		if slices.Contains(envNames, match[1]) {
			continue
		}
		envNames = append(envNames, match[1])
	}

	return envNames
}

// substituteEnv substitute envName in data when encased in a set of delimiters appling transformations on the
// value contained in it.
func substituteEnv(data, envName string, prefixes []string) (string, error) {
	doubleQouted := doubleQoutedLeftDelim + envName + doubleQoutedRightDelim
	substitution, err := valueForEnv(envName, prefixes, func(str string) string {
		str = strconv.Quote(str)
		return strings.ReplaceAll(str, `\\`, `\`)
	})
	if err != nil {
		return "", err
	}

	data = strings.ReplaceAll(data, doubleQouted, substitution)

	singleQouted := singleQoutedLeftDelim + envName + singleQoutedRightDelim
	substitution, err = valueForEnv(envName, prefixes, func(str string) string {
		str = strconv.Quote(str)
		str = strings.ReplaceAll(str, `\\`, `\`)
		str = strings.ReplaceAll(str, `\"`, `"`)
		return "'" + str[1:len(str)-1] + "'"
	})
	if err != nil {
		return "", err
	}

	data = strings.ReplaceAll(data, singleQouted, substitution)

	unquoted := unqutedLeftDelim + envName + unqutedRightDelim
	substitution, err = valueForEnv(envName, prefixes, func(str string) string {
		return strings.ReplaceAll(str, "\n", "\\n") // keep multiline string on one line
	})
	if err != nil {
		return "", err
	}

	return strings.ReplaceAll(data, unquoted, substitution), nil
}

func valueForEnv(envName string, prefixes []string, fn func(string) string) (string, error) {
	envsToCheck := make([]string, 0, len(prefixes)+1)
	for _, prefix := range prefixes {
		envsToCheck = append(envsToCheck, prefix+envName)
	}
	envsToCheck = append(envsToCheck, envName)

	for _, envName := range envsToCheck {
		if val, exists := os.LookupEnv(envName); exists {
			return fn(val), nil
		}
	}

	return "", fmt.Errorf("environment variable %q not found", envName)
}
