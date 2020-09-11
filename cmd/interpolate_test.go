package cmd

import (
	"bytes"
	"os"
	"testing"
)

func initPrefix() {
	primary_prefix = "MIA"
	alternative_prefix = "DEV"
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

	if !bytes.Equal(out, res) {
		t.Errorf("output not correct \nexpected: %s \nfound: %s", string(out), string(res))
	}
}

func TestDollar(t *testing.T) {
	initPrefix()

	os.Setenv("MIA_FIRST_ENV", "$contains$dollars$")
	defer os.Unsetenv("MIA_FIRST_ENV")

	in := []byte(`
  "first": "aaa",
  "second": "{{FIRST_ENV}}",
  "third": "aaa",
  `)
	out := []byte(`
  "first": "aaa",
  "second": "$contains$dollars$",
  "third": "aaa",
  `)

	res := interpolate(in)

	if !bytes.Equal(out, res) {
		t.Errorf("output not correct \nexpected: %s \nfound: %s", string(out), string(res))
	}
}

func TestNewLines(t *testing.T) {
	initPrefix()

	os.Setenv("MIA_FIRST_ENV", `envfirstline\nenvsecondline\nenvthirdline\n`)
	os.Setenv("DEV_SECOND_ENV", "envfirstline\nenvsecondline\nenvthirdline\n")
	defer os.Unsetenv("MIA_FIRST_ENV")
	defer os.Unsetenv("DEV_SECOND_ENV")

	in := []byte(`
  "first": "aaa",
  "second": "{{FIRST_ENV}}",
  "third": "{{SECOND_ENV}}",
  `)
	out := []byte(`
  "first": "aaa",
  "second": "envfirstline\nenvsecondline\nenvthirdline\n",
  "third": "envfirstline\nenvsecondline\nenvthirdline\n",
  `)

	res := interpolate(in)

	if !bytes.Equal(out, res) {
		t.Errorf("output not correct \nexpected: %s \nfound: %s", out, res)
	}
}
