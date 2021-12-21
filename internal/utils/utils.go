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

package utils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/afero"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/cli-runtime/pkg/printers"
)

// DeployConfig are all the specific configurations used in deploy phase
type DeployConfig struct {
	DeployType              string
	ForceDeployWhenNoSemver bool
	EnsureNamespace         bool
}

// Options global option for the cli that can be passed to all commands
type Options struct {
	Config *genericclioptions.ConfigFlags

	CertificateAuthority  string
	ClientCertificate     string
	ClientKey             string
	Cluster               string
	Context               string
	Kubeconfig            string
	InsecureSkipTLSVerify bool
	Namespace             string
	Server                string
	Token                 string
	User                  string
}

const (
	StdinToken string = "-"
)

// fs return the file system to use by default (override it for tests)
var fs = &afero.Afero{Fs: afero.NewOsFs()}

// CheckError default error handling function
func CheckError(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

// ExtractYAMLFiles return array of YAML filenames from array of files and directories
func ExtractYAMLFiles(paths []string) ([]string, error) {
	if len(paths) == 1 && paths[0] == StdinToken {
		return []string{StdinToken}, nil
	}

	fileNames := []string{}

	// Extract files from directories
	for _, path := range paths {
		// get absolute path for good measure
		globalPath, err := filepath.Abs(path)
		if err != nil {
			return nil, err
		}

		pathIsDirectory, err := fs.IsDir(globalPath)
		if err != nil {
			fmt.Printf("WARN: can't read input file at path %s\n", globalPath)
			continue
		}

		if pathIsDirectory {
			pathsInDirectory, err := extractYAMLFromDir(globalPath)
			if err != nil {
				return nil, err
			}

			fileNames = append(fileNames, pathsInDirectory...)
		} else if isYAMLFile(globalPath) {
			fileNames = append(fileNames, globalPath)
		}
	}
	return fileNames, nil
}

// extractYAMLFromDir extracts from a directory the global path to YAML files contained in it.
// This function does not look into subdirs.
func extractYAMLFromDir(directoryPath string) ([]string, error) {
	filesPath := []string{}

	files, err := fs.ReadDir(directoryPath)
	if err != nil {
		return nil, err
	}

	for _, path := range files {
		if !path.IsDir() && isYAMLFile(path.Name()) {
			filesPath = append(filesPath, filepath.Join(directoryPath, path.Name()))
		}
	}

	return filesPath, nil
}

// isYAMLFile this function return true if the path contain a known YAML extension
func isYAMLFile(path string) bool {
	fileExtension := filepath.Ext(path)
	return fileExtension == ".yaml" || fileExtension == ".yml"
}

// WriteYamlsToDisk marshals and writes kubernetes runtime objects to YAML file
func WriteYamlsToDisk(objs map[string]runtime.Object, outputDirectory string) {
	printer := &printers.YAMLPrinter{}
	for yamlName, obj := range objs {
		fileName := outputDirectory + "/" + yamlName + ".yaml"
		file, err := CreateFile(fileName)
		CheckError(err)
		printer.PrintObj(obj, file)
	}
}

// ReadFile read a file from the file system
func ReadFile(filename string) ([]byte, error) {
	return fs.ReadFile(filename)
}

// MkdirAll create a folder
func MkdirAll(name string) error {
	return fs.MkdirAll(name, os.FileMode(0755))
}

// RemoveAll removes a directory path and any children it contains.
func RemoveAll(path string) error {
	return fs.RemoveAll(path)
}

// CreateFile create a new file in path
func CreateFile(path string) (afero.File, error) {
	return fs.Create(path)
}

// WriteFile write data to file
func WriteFile(filename string, data []byte) error {
	return fs.WriteFile(filename, data, os.FileMode(0644))
}
