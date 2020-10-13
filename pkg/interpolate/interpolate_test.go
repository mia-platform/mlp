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

package interpolate

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var re = "{{([A-Z0-9_]+)}}"
var prefixes = []string{"DEV_"}

func TestEmptyVariable(t *testing.T) {
	os.Setenv("FIRST_ENV", "test")
	defer os.Unsetenv("FIRST_ENV")

	in := []byte(`
  "first": "{{FIRST_ENV}}",
  "second": "{{}}",
  `)
	expout := []byte(`
  "first": "test",
  "second": "{{}}",
  `)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, expout, out, "the regex should not match anything inside empty brackets")
}

func TestDollar(t *testing.T) {
	os.Setenv("FIRST_ENV", "$contains$dollars$")
	defer os.Unsetenv("FIRST_ENV")

	in := []byte(`
  "first": "field",
  "second": "{{FIRST_ENV}}",
  "third": "field",
  `)
	expout := []byte(`
  "first": "field",
  "second": "$contains$dollars$",
  "third": "field",
  `)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, expout, out, "they should be equal")
}

func TestNewLines(t *testing.T) {
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
	expout := []byte(`
  "first": "field",
  "second": "{\n    \"first\": \"field\",\n    \"second\": \"field\",\n    \"third\": \"field\",\n    \"fourth\": \"field\"\n  }",
  `)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, expout, out, "new lines should not be consumed while manipulating strings")

}

func TestSpecialChars(t *testing.T) {
	os.Setenv("DEV_SECOND_ENV", "env\\firstline\nenv\tsecondline\nenvthirdline\n")

	defer os.Unsetenv("DEV_SECOND_ENV")

	in := []byte(`
  "first": "field",
  "second": "{{SECOND_ENV}}",
  `)
	expout := []byte(`
  "first": "field",
  "second": "env\\firstline\nenv\tsecondline\nenvthirdline\n",
  `)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, expout, out, "special chars should not be consumed while manipulating strings")
}

func TestVarNotInParenthesis(t *testing.T) {
	in := []byte(`
  "first": "field",
  "second": "SECOND_ENV",
	`)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, in, out, "an existing environment variable which is not inside {{}} should not be interpolated")
}

func TestVarWithSpaces(t *testing.T) {
	in := []byte(`
  "second": "{{ SECOND_ENV }}",
  "second": "{{SECOND_ENV }}",
  "second": "{{ SECOND_ENV}}",
  "second": "{{SECOND _ENV}}",
  "second": "{{ SECOND _ENV }}",
	`)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, in, out, "No variables with spaces should be matched by the regex")
}

func TestNonExistingVar(t *testing.T) {
	os.Setenv("DEV_FIRST_ENV", "first")
	defer os.Unsetenv("DEV_FIRST_ENV")

	envs := make(map[string]envVar)
	envs["FIRST_ENV"] = envVar{name: "{{FIRST_ENV}}"}
	envs["SECOND_ENV"] = envVar{name: "{{SECOND_ENV}}"}

	errMsg := "Environment Variable SECOND_ENV: not found"
	require.EqualError(t, checkEnvs(envs, prefixes), errMsg)
}
