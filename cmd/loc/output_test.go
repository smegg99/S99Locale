// cmd/loc/output_test.go

package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A piped stderr carries no escapes.
func TestAPipedRunCarriesNoEscapes(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})

	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	a.close()

	if got := out.String(); strings.Contains(got, "\x1b") {
		t.Fatalf("an escape reached a non-terminal writer: %q", got)
	}
}

// State is a glyph and a word as well as a colour, so a result reads the same without colour.
func TestASuccessLineCarriesItsGlyphAndWord(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})

	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	a.close()

	got := ansi.Strip(out.String())
	if !strings.Contains(got, "[+] ok") {
		t.Fatalf("no state on the success line:\n%s", got)
	}
	if !strings.Contains(got, "generated") {
		t.Fatalf("no label on the success line:\n%s", got)
	}
}

// A failing check names each file's class, because `loc build` cannot fix one of them.
func TestAFailingCheckNamesBothClasses(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})
	ctx := context.Background()

	if err := cmdCheck(ctx, a, false); err == nil {
		t.Fatal("check on an unbuilt project passed")
	}
	a.close()

	got := ansi.Strip(out.String())
	if !strings.Contains(got, "[~] ") {
		t.Fatalf("stale files are not marked stale:\n%s", got)
	}
}

// --color exists on the root and accepts the three values its usage names.
func TestColorIsOnTheRootAndTakesThreeValues(t *testing.T) {
	root := rootCmd(newApp(&bytes.Buffer{}, &bytes.Buffer{}, []string{"LANG=C"}))
	if root.PersistentFlags().Lookup("color") == nil {
		t.Fatal("there is no --color flag")
	}

	for _, value := range []string{"auto", "always", "never"} {
		if err := root.PersistentFlags().Set("color", value); err != nil {
			t.Errorf("--color %s: %v", value, err)
		}
	}
	if err := root.PersistentFlags().Set("color", "sometimes"); err == nil {
		t.Fatal("--color sometimes was accepted")
	}
}
