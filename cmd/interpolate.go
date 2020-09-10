/*
Copyright © 2020 NAME HERE <EMAIL ADDRESS>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package cmd

import (
	"errors"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

var primary_prefix string
var alternative_prefix string
var file_path string

// interpolateCmd represents the interpolate command
var interpolateCmd = &cobra.Command{
	Use: "interpolate",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) < 1 {
			return errors.New("no file specified")
		}
		_, err := os.Stat(args[0])
		if os.IsNotExist(err) {
			return errors.New("file " + args[0] + " does not exists")
		}

		return nil
	},
	Short: "Interpolate variables in file",
	Long:  `Interpolate the environment variables inside {{}} in file and substitutes them with the actual value of the variables`,
	RunE: func(cmd *cobra.Command, args []string) error {
		file_path := args[0]
		file, err := ioutil.ReadFile(file_path)
		check(err)

		interpolated_file := interpolate(file)

		f, err := os.Create("out-" + file_path)
		check(err)
		defer f.Close()

		_, err = f.Write(interpolated_file)
		check(err)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(interpolateCmd)

	interpolateCmd.Flags().StringVarP(&primary_prefix, "prefix", "p", "", "primary prefix to add when looking for envs")
	interpolateCmd.Flags().StringVarP(&alternative_prefix, "alternative-prefix", "a", "", "prefix to use when the primary prefix env does not exists")
}

type env_var struct {
	name  string
	value string
}

func interpolate(file []byte) []byte {

	envs := getVariablesToInterpolate(file)

	//exit if there are no variables to interpolate
	if len(envs) == 0 {
		os.Exit(0)
	}

	err := checkEnvs(envs)
	check(err)

	interpolated_file := interpolateVariables(file, envs)

	return interpolated_file
}

func check(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func getVariablesToInterpolate(file_content []byte) map[string]*env_var {
	re := regexp.MustCompile("\\{\\{(.+?)\\}\\}")
	match := re.FindAllStringSubmatch(string(file_content), -1)

	vars := make(map[string]*env_var)
	for parsed_var := range match {
		var_name := strings.ReplaceAll(match[parsed_var][1], " ", "")
		//keep track of the entire pattern found by the regex
		//using as key the variable name
		if _, exists := vars[var_name]; !exists {
			vars[var_name] = &env_var{name: match[parsed_var][0]}
		}
	}
	return vars
}

func checkEnvs(envs map[string]*env_var) error {
	for var_name, _ := range envs {

		var_prefixed := primary_prefix + "_" + var_name
		var_prefixed_alternative := alternative_prefix + "_" + var_name
		env := envs[var_name]

		if os.Getenv(var_prefixed) != "" {
			(*env).value = os.Getenv(var_prefixed)
		} else if os.Getenv(var_prefixed_alternative) != "" {
			(*env).value = os.Getenv(var_prefixed_alternative)
		} else {
			return errors.New("environment variables " + var_prefixed + " and " + var_prefixed_alternative + " do not exist")
		}
	}
	return nil
}

func interpolateVariables(file []byte, envs map[string]*env_var) []byte {
	file_string := string(file)

	for var_name, _ := range envs {
		env := *envs[var_name]
		file_string = strings.ReplaceAll(file_string, env.name, env.value)
	}

	return []byte(file_string)
}
