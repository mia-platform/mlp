package cmd

import (
	"bytes"
	"os"
	"testing"
)

func initPrefix() {
	primaryPrefix = "MIA"
	alternativePrefix = "DEV"
}

func checkResult(expected []byte, output []byte, t *testing.T) {
	if !bytes.Equal(expected, output) {
		t.Errorf("output not correct \nexpected: %s \nfound: %s", expected, output)
	}
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

	res := interpolate(in)

	checkResult(out, res, t)
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

	res := interpolate(in)

	checkResult(out, res, t)
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

	res := interpolate(in)

	checkResult(out, res, t)

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

	res := interpolate(in)

	checkResult(out, res, t)

}
