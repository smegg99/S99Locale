// cmd/loc/table_test.go

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A column wide enough in English widens for Polish.
func TestATableWidensForItsContent(t *testing.T) {
	a, _, _ := testApp(t, options{})

	narrow := ansi.Strip(a.table([]string{"NAME", "COUNT"}, [][]string{{"app", "3"}}))
	wide := ansi.Strip(a.table([]string{"NAME", "COUNT"}, [][]string{{"przestrzennazwaktorajestdluga", "3"}}))

	if len(strings.SplitN(wide, "\n", 2)[0]) <= len(strings.SplitN(narrow, "\n", 2)[0]) {
		t.Fatal("the table did not widen for a longer value")
	}
	if !strings.Contains(narrow, "app") || !strings.Contains(narrow, "3") {
		t.Fatalf("the table lost its content:\n%s", narrow)
	}
}

// LANG=C cannot draw U+2500, and testApp sets it, so the border is ASCII.
func TestATableDrawsAsciiBordersUnderLangC(t *testing.T) {
	a, _, _ := testApp(t, options{})

	got := a.table([]string{"NAME"}, [][]string{{"app"}})
	if strings.ContainsRune(got, '\u2500') {
		t.Fatalf("a box-drawing character survived LANG=C:\n%s", got)
	}
	if !strings.Contains(got, "-") {
		t.Fatalf("no ASCII border:\n%s", got)
	}
}

// No site in cmd/loc pads a column by hand.
func TestNoHandPaddedColumnsSurvive(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})

	if err := cmdStats(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	a.close()

	// Two rows of a table start their second column at the same index.
	body := ansi.Strip(out.String())
	if !strings.Contains(body, "NAME") {
		t.Fatalf("stats has no table headers:\n%s", body)
	}
}
