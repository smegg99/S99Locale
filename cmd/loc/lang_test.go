// cmd/loc/lang_test.go

package main

import (
	"bytes"
	"strings"
	"testing"
)

// pinEnglish makes rootCmd detect English; LOC_LANG wins over LC_ALL, LC_MESSAGES and LANG.
func pinEnglish(t *testing.T) {
	t.Helper()
	t.Setenv("LOC_LANG", "en")
}

func TestLangRendersHelpInPolish(t *testing.T) {
	pinEnglish(t)

	out := &bytes.Buffer{}
	root := rootCmd(newApp(&bytes.Buffer{}, &bytes.Buffer{}))
	root.SetArgs([]string{"--lang", "pl", "--help"})
	root.SetOut(out)
	root.SetErr(out)

	if err := root.Execute(); err != nil {
		t.Fatalf("err = %v\n%s", err, out.String())
	}
	for _, want := range []string{"Użycie:", "Dostępne polecenia:", "kompiluje kanoniczny YAML"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("help does not contain %q:\n%s", want, out.String())
		}
	}
}

// --help before --lang pl still renders Polish help.
func TestLangSurvivesHelpComingFirst(t *testing.T) {
	pinEnglish(t)

	out := &bytes.Buffer{}
	root := rootCmd(newApp(&bytes.Buffer{}, &bytes.Buffer{}))
	root.SetArgs([]string{"--help", "--lang", "pl"})
	root.SetOut(out)
	root.SetErr(out)

	if err := root.Execute(); err != nil {
		t.Fatalf("err = %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Użycie:") {
		t.Errorf("help is not Polish:\n%s", out.String())
	}
}

// An unsupported --lang is a usage error; an unsupported LOC_LANG is not.
func TestUnsupportedLangIsAUsageError(t *testing.T) {
	pinEnglish(t)

	out := &bytes.Buffer{}
	root := rootCmd(newApp(&bytes.Buffer{}, &bytes.Buffer{}))
	root.SetArgs([]string{"--lang", "de"})
	root.SetOut(out)
	root.SetErr(out)

	err := root.Execute()
	if err == nil {
		t.Fatal("--lang de was accepted")
	}
	if !strings.Contains(err.Error(), "unsupported language de") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(err.Error(), "en, pl") {
		t.Fatalf("the error does not say what loc speaks: %v", err)
	}
}

// Cobra's own commands and flags are text loc prints, so loc translates them.
func TestCobrasOwnTextIsTranslated(t *testing.T) {
	pinEnglish(t)

	out := &bytes.Buffer{}
	root := rootCmd(newApp(&bytes.Buffer{}, &bytes.Buffer{}))
	root.SetArgs([]string{"--lang", "pl", "--help"})
	root.SetOut(out)
	root.SetErr(out)

	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, banned := range []string{"Available Commands:", "Help about any command", "help for loc"} {
		if strings.Contains(body, banned) {
			t.Errorf("English survived in Polish help: %q\n%s", banned, body)
		}
	}
}

// Args validation is text loc prints too; cobra.NoArgs says it in English.
func TestTooManyArgumentsIsAnsweredInTheReadersLanguage(t *testing.T) {
	pinEnglish(t)

	out := &bytes.Buffer{}
	root := rootCmd(newApp(&bytes.Buffer{}, &bytes.Buffer{}))
	root.SetArgs([]string{"--lang", "pl", "build", "extra"})
	root.SetOut(out)
	root.SetErr(out)

	err := root.Execute()
	if err == nil {
		t.Fatal("build accepted a positional argument")
	}
	if !strings.Contains(err.Error(), "nie przyjmuje argumentów") {
		t.Fatalf("err = %v", err)
	}
}
