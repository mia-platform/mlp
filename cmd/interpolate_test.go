package cmd

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func initPrefix() {
	primaryPrefix = "DEV"
	alternativePrefix = "MIA"
}

func TestEmptyVariable(t *testing.T) {
	initPrefix()

	os.Setenv("MIA_FIRST_ENV", "test")
	defer os.Unsetenv("MIA_FIRST_ENV")

	in := []byte(`
  "first": "{{FIRST_ENV}}",
  "second": "{{}}",
  `)
	out := []byte(`
  "first": "test",
  "second": "{{}}",
  `)

	require.Equal(t, out, interpolate(in), "the regex should not match anything inside empty brackets")
}

func TestDollar(t *testing.T) {
	initPrefix()

	os.Setenv("MIA_FIRST_ENV", "$contains$dollars$")
	defer os.Unsetenv("MIA_FIRST_ENV")

	in := []byte(`
  "first": "field",
  "second": "{{FIRST_ENV}}",
  "third": "field",
  `)
	out := []byte(`
  "first": "field",
  "second": "$contains$dollars$",
  "third": "field",
  `)

	require.Equal(t, out, interpolate(in), "they should be equal")
}

func TestNewLines(t *testing.T) {
	initPrefix()

	os.Setenv("DEV_SECOND_ENV", `{
    "first": "field",
    "second": "field",
    "third": "field",
    "fourth": "field"
  }`)

	defer os.Unsetenv("DEV_SECOND_ENV")

	in := []byte(`
  "first": "field",
  "second": "{{SECOND_ENV}}",
  `)
	out := []byte(`
  "first": "field",
  "second": "{\n    \"first\": \"field\",\n    \"second\": \"field\",\n    \"third\": \"field\",\n    \"fourth\": \"field\"\n  }",
  `)

	require.Equal(t, out, interpolate(in), "new lines should not be consumed while manipulating strings")

}

func TestSpecialChars(t *testing.T) {
	initPrefix()

	os.Setenv("DEV_SECOND_ENV", "env\\firstline\nenv\tsecondline\nenvthirdline\n")

	defer os.Unsetenv("DEV_SECOND_ENV")

	in := []byte(`
  "first": "field",
  "second": "{{SECOND_ENV}}",
  `)
	out := []byte(`
  "first": "field",
  "second": "env\\firstline\nenv\tsecondline\nenvthirdline\n",
  `)

	require.Equal(t, out, interpolate(in), "special chars should not be consumed while manipulating strings")
}

func TestVarNotInParenthesis(t *testing.T) {
	initPrefix()

	in := []byte(`
  "first": "field",
  "second": "SECOND_ENV",
  `)

	require.Nil(t, interpolate(in), "an existing environment variable which is not inside {{}} should not be interpolated")
}

func TestVarWithSpaces(t *testing.T) {
	initPrefix()

	in := []byte(`
  "second": "{{ SECOND_ENV }}",
  "second": "{{SECOND_ENV }}",
  "second": "{{ SECOND_ENV}}",
  "second": "{{SECOND _ENV}}",
  "second": "{{ SECOND _ENV }}",
  `)

	require.Nil(t, interpolate(in), "No variables with spaces should be matched by the regex")
}

func TestNonExistingVar(t *testing.T) {
	initPrefix()

	os.Setenv("DEV_FIRST_ENV", "first")
	defer os.Unsetenv("DEV_FIRST_ENV")

	envs := make(map[string]envVar)
	envs["FIRST_ENV"] = envVar{name: "{{FIRST_ENV}}"}
	envs["SECOND_ENV"] = envVar{name: "{{SECOND_ENV}}"}

	errMsg := errors.New("environment variables DEV_SECOND_ENV and MIA_SECOND_ENV do not exist")
	require.Error(t, errMsg, func() { checkEnvs(envs) }, "should return error if a variable does not exists")
}
