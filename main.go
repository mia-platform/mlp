package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
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

	fmt.Println("primary prefix: ", *primary_prefix)
	fmt.Println("fallback prefix: ", *fallback_prefix)
	fmt.Println("File to interpolate: ", *file_path)

	file, err := ioutil.ReadFile(*file_path)
	check(err)

	vars := getVariablesToInterpolate(file)

	envs, err := checkEnvs(vars)
	check(err)
	fmt.Println(envs)
}

func check(err error) {
	if err != nil {
		fmt.Println("Error occurred: ", err)
		os.Exit(1)
	}
}

func getVariablesToInterpolate(file_content []byte) []string {
	re := regexp.MustCompile("\\$\\{(.*?)\\}")
	match := re.FindAllStringSubmatch(string(file_content), -1)

	vars := make([]string, 0)
	for parsed_var := range match {
		vars = append(vars, match[parsed_var][1])
	}
	return vars
}

func checkEnvs(vars []string) (map[string]string, error) {
	envs := make(map[string]string, 0)
	for index := range vars {

		var_prefixed := *primary_prefix + "_" + vars[index]
		var_prefixed_fallback := *fallback_prefix + "_" + vars[index]

		fmt.Println(envs)

		if os.Getenv(var_prefixed) != "" {
			envs[vars[index]] = os.Getenv(var_prefixed)
		} else if os.Getenv(var_prefixed_fallback) != "" {
			envs[vars[index]] = os.Getenv(var_prefixed_fallback)
		} else {
			return nil, errors.New("environment variables " + vars[index] + " does not exist")
		}
	}

	return envs, nil
}
