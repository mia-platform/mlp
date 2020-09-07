package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"
)

var primary_prefix = flag.String("prefix", "", "primary prefix appended to pick env vars")
var fallback_prefix = flag.String("fallback-prefix", "", "prefix used when variables with the primary prefix does not existas")
var file_path = flag.String("file", "", "file to interpolate")

func main() {

	flag.Parse()

	if *primary_prefix == "" || *fallback_prefix == "" || *file_path == "" {
		fmt.Println("Usage: -prefix PREFIX -fallback-prefix FALLBACK_PREFIX file")
		os.Exit(1)
	}

	file, err := ioutil.ReadFile(*file_path)
	check(err)

	vars := getVariablesToInterpolate(file)

	//exit if there are no variables to interpolate
	if len(vars) == 0 {
		os.Exit(0)
	}

	envs, err := checkEnvs(vars)
	check(err)

	interpolateVariables(file, vars, envs)
}

func check(err error) {
	if err != nil {
		fmt.Println("Error occurred: ", err)
		os.Exit(1)
	}
}

func getVariablesToInterpolate(file_content []byte) map[string]string {
	re := regexp.MustCompile("\\{\\{(.+?)\\}\\}")
	match := re.FindAllStringSubmatch(string(file_content), -1)

	vars := make(map[string]string, 0)
	for parsed_var := range match {
		//keep track of the entire pattern found by the regex
		//using as key the variable name
		vars[match[parsed_var][1]] = match[parsed_var][0]
	}
	return vars
}

func checkEnvs(vars map[string]string) (map[string]string, error) {
	envs := make(map[string]string, 0)
	for var_name := range vars {

		var_prefixed := *primary_prefix + "_" + var_name
		var_prefixed_fallback := *fallback_prefix + "_" + var_name

		fmt.Println(envs)

		if os.Getenv(var_prefixed) != "" {
			envs[var_name] = os.Getenv(var_prefixed)
		} else if os.Getenv(var_prefixed_fallback) != "" {
			envs[var_name] = os.Getenv(var_prefixed_fallback)
		} else {
			return nil, errors.New("environment variables " + var_name + " does not exist")
		}
	}

	return envs, nil
}

func interpolateVariables(file []byte, vars map[string]string, envs map[string]string) bool {
	file_string := string(file)

	for var_name, _ := range envs {
		file_string = strings.ReplaceAll(file_string, vars[var_name], envs[var_name])
	}

	fmt.Println(file_string)
	f, err := os.Create("out-" + *file_path)
	check(err)

	written_bytes, err := f.WriteString(file_string)
	fmt.Println("Successfully written ", written_bytes, " bytes")
	check(err)
	err = f.Close()
	check(err)
	return true
}
