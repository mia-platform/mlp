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
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

var re = "{{([A-Z0-9_]+)}}"
var prefixes = []string{"DEV_"}

func TestEmptyVariable(t *testing.T) {
	t.Setenv("FIRST_ENV", "test")

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
	t.Setenv("FIRST_ENV", "$contains$dollars$")

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
	t.Setenv("DEV_SECOND_ENV", `{
    "first": "field",
    "second": "field",
    "third": "field",
    "fourth": "field"
  }`)

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
	require.Equal(t, string(expout), string(out), "new lines should not be consumed while manipulating strings")
}

func TestEscapeQuote(t *testing.T) {
	t.Setenv("DEV_SECOND_ENV", `{ "foo": "bar" }`)
	t.Setenv("DEV_STRING_ESCAPED_ENV", `abd\def`)
	t.Setenv("JSON_ESCAPED_ENV", `{ \"foo\": \"bar\" }`)
	t.Setenv("NAMESPACE", `my-namespace`)
	t.Setenv("DEV_THIRD_ENV", `{ "foo": "bar\ntaz" }`)

	in := []byte(`
	"noEscape_singleQuote": '{{SECOND_ENV}}',

	"escape_singleQuoteMiddle": 'abc{{SECOND_ENV}}def',
	"escape_singleQuoteMiddle2": '{{STRING_ESCAPED_ENV}}',
	"escape_doubleQuote": "{{SECOND_ENV}}",
	"escape_doubleQuoteMiddle": "abc{{JSON_ESCAPED_ENV}}def",
	"escape_string": "{{NAMESPACE}}.svc.cluster.local",
	"escape_noQuote": {{SECOND_ENV}},
	"escape_noQuoteMiddle": abc{{SECOND_ENV}}def,
	"escape_doubleQuoteNewline": {{THIRD_ENV}},
	`)
	expout := []byte(`
	"noEscape_singleQuote": '{ "foo": "bar" }',

	"escape_singleQuoteMiddle": 'abc{ "foo": "bar" }def',
	"escape_singleQuoteMiddle2": 'abd\def',
	"escape_doubleQuote": "{ \"foo\": \"bar\" }",
	"escape_doubleQuoteMiddle": "abc{ \"foo\": \"bar\" }def",
	"escape_string": "my-namespace.svc.cluster.local",
	"escape_noQuote": { "foo": "bar" },
	"escape_noQuoteMiddle": abc{ "foo": "bar" }def,
	"escape_doubleQuoteNewline": { "foo": "bar\ntaz" },
	`)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, string(expout), string(out), "double quote should be escaped only inside double quote")
}

func TestSpecialChars(t *testing.T) {
	t.Setenv("DEV_FIRST_ENV", `env\\first\line`)
	t.Setenv("DEV_SECOND_ENV", "env\\\\first\\line\nenv\tsecondline\nenvthirdline\n")

	in := []byte(`
  "first": "field",
  "second": "{{SECOND_ENV}}",
  "third": '{{SECOND_ENV}}',
  "fourth": "{{FIRST_ENV}}",
  "fifth": '{{FIRST_ENV}}'
  `)
	expout := []byte(`
  "first": "field",
  "second": "env\\first\line\nenv\tsecondline\nenvthirdline\n",
  "third": 'env\\first\line\nenv\tsecondline\nenvthirdline\n',
  "fourth": "env\\first\line",
  "fifth": 'env\\first\line'
  `)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, string(expout), string(out), "special chars should not be consumed while manipulating strings")
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
	t.Setenv("DEV_FIRST_ENV", "first")

	envs := make(map[string]envVar)
	envs["FIRST_ENV"] = envVar{name: "{{FIRST_ENV}}"}
	envs["SECOND_ENV"] = envVar{name: "{{SECOND_ENV}}"}

	errMsg := "Environment Variable SECOND_ENV: not found"
	require.EqualError(t, checkEnvs(envs, prefixes), errMsg)
}

func TestStringifiedObjectWithSingleApex(t *testing.T) {
	t.Setenv("DEV_SECOND_ENV", `{"piatti-json":"platform-development.development.piatti-json"}`)

	in := []byte(`
  "first": 'field',
  "second": '{{SECOND_ENV}}',
  `)
	expout := []byte(`
  "first": 'field',
  "second": '{"piatti-json":"platform-development.development.piatti-json"}',
  `)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, string(expout), string(out))
}

func TestJSONConfigmap(t *testing.T) {
	t.Setenv("DEV_THIRD_ENV", `{"type":"a type","project_id":"something","private_key_id":"my-key","private_key":"-----BEGIN PRIVATE KEY-----\nfooo\nbar\n-----END PRIVATE KEY-----\n","client_email":"my@email.com","client_id":"client-id","auth_uri":"https://auth-uri.com","token_uri":"https://auth-uri.com","auth_provider_x509_cert_url":"https://api.com/certs","client_x509_cert_url":"https://api.com/certs/fooo%40bar"}`)

	in, err := os.ReadFile("testdata/file.json")
	require.NoError(t, err)

	expout, err := os.ReadFile("testdata/outFile.json")
	require.NoError(t, err)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, string(expout), string(out))
}

func TestJSONWithEscapes(t *testing.T) {
	t.Setenv("DEV_THIRD_ENV", `"{\"type\":\"a type\"}"`)

	in, err := os.ReadFile("testdata/file.json")
	require.NoError(t, err)

	expout := `"{\"type\":\"a type\"}"`

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, expout, string(out))
}

func TestCertificateInString(t *testing.T) {
	t.Setenv("DEV_SECOND_ENV", `-----BEGIN RSA PRIVATE KEY-----
XXXXXXXXXXXXXXXXXXXXXXXXX
YYYYYYYYYYYYYYYYYYYYYYYYYY
ZZZZZZZZZZZZZZZZZZZZZZZZZZ
-----END RSA PRIVATE KEY-----`)

	in := []byte(`
  "doubleQuote": "{{SECOND_ENV}}"
  `)
	expout := []byte(`
  "doubleQuote": "-----BEGIN RSA PRIVATE KEY-----\nXXXXXXXXXXXXXXXXXXXXXXXXX\nYYYYYYYYYYYYYYYYYYYYYYYYYY\nZZZZZZZZZZZZZZZZZZZZZZZZZZ\n-----END RSA PRIVATE KEY-----"
  `)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, string(expout), string(out))
}

func TestCertificateInFile(t *testing.T) {
	t.Setenv("SECOND_ENV", `-----BEGIN RSA PRIVATE KEY-----
XXXXXXXXXXXXXXXXXXXXXXXXX
YYYYYYYYYYYYYYYYYYYYYYYYYY
ZZZZZZZZZZZZZZZZZZZZZZZZZZ
-----END RSA PRIVATE KEY-----`)

	in := []byte(`
{{SECOND_ENV}}
`)
	expout := []byte(`
-----BEGIN RSA PRIVATE KEY-----
XXXXXXXXXXXXXXXXXXXXXXXXX
YYYYYYYYYYYYYYYYYYYYYYYYYY
ZZZZZZZZZZZZZZZZZZZZZZZZZZ
-----END RSA PRIVATE KEY-----
`)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, string(expout), string(out))
}

func TestConvertReplicas(t *testing.T) {
	t.Setenv("MY_REPLICAS", `4`)

	in, err := os.ReadFile("testdata/deployment_tointerpolate.yaml")
	require.NoError(t, err)

	expout, err := os.ReadFile("testdata/deployment_interpolated.yaml")
	require.NoError(t, err)

	out, err := Interpolate(in, prefixes, re)
	require.Nil(t, err)
	require.Equal(t, string(expout), string(out))
}
