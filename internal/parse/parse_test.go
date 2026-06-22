// internal/parse/parse_test.go

package parse_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
)

// root writes a locale tree and returns its path; files maps relative path to contents.
func root(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func parseOne(t *testing.T, files map[string]string, locales ...string) *parse.Result {
	t.Helper()
	if len(locales) == 0 {
		locales = []string{"en"}
	}
	res, err := parse.Roots(context.Background(), []string{root(t, files)}, locales)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func mustGet(t *testing.T, res *parse.Result, locale, key string) *message.Message {
	t.Helper()
	m, ok := res.Set.Get(locale, key)
	if !ok {
		t.Fatalf("key %q missing; have %v", key, res.Set.Keys())
	}
	return m
}

func noErrors(t *testing.T, res *parse.Result) {
	t.Helper()
	for _, e := range res.Errors {
		t.Errorf("unexpected: %v", e)
	}
	if t.Failed() {
		t.FailNow()
	}
}

// The path is the namespace. Files are never flattened.
func TestNestedPathFormsTheKey(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/auth/login.yaml": "title:\n  value: \"Sign in\"\n",
		"en/common.yaml":     "cancel:\n  value: \"Cancel\"\n",
	})
	noErrors(t, res)

	if m := mustGet(t, res, "en", "auth.login.title"); m.Value != "Sign in" {
		t.Fatalf("value = %q", m.Value)
	}
	if m := mustGet(t, res, "en", "common.cancel"); m.Namespace != "common" {
		t.Fatalf("namespace = %q, want common", m.Namespace)
	}
	if m := mustGet(t, res, "en", "auth.login.title"); m.Namespace != "auth" {
		t.Fatalf("namespace = %q, want auth from the first segment", m.Namespace)
	}
}

func TestGroupsNestWithoutBecomingMessages(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/common.yaml": `
action:
  accept:
    value: "OK"
  cancel:
    value: "Cancel"
`})
	noErrors(t, res)
	mustGet(t, res, "en", "common.action.accept")
	mustGet(t, res, "en", "common.action.cancel")
}

func TestPlainMessageMetadata(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/auth.yaml": `
loginFailed:
  value: "Could not sign in as {email}"
  description: "After a failed login"
  targets: [web, backend]
  placeholders:
    email: {type: string}
  sharing:
    isolate: true
    reason: "wording diverges"
`})
	noErrors(t, res)

	m := mustGet(t, res, "en", "auth.loginFailed")
	if m.Kind != message.KindPlain {
		t.Fatalf("kind = %v", m.Kind)
	}
	if m.Description != "After a failed login" {
		t.Fatalf("metadata lost: %+v", m)
	}
	if len(m.Targets) != 2 || m.Targets[0] != message.TargetBackend {
		t.Fatalf("targets = %v, want them sorted", m.Targets)
	}
	if m.Placeholders["email"].Type != message.TypeString {
		t.Fatalf("placeholder = %+v", m.Placeholders)
	}
	if !m.Sharing.Isolate || m.Sharing.Reason == "" {
		t.Fatalf("sharing = %+v", m.Sharing)
	}
}

func TestPluralWithAllPolishBands(t *testing.T) {
	res := parseOne(t, map[string]string{
		"pl/tuner.yaml": `
reeds:
  plural:
    argument: count
    forms:
      zero: "brak głosów"
      one: "{count} głos"
      few: "{count} głosy"
      many: "{count} głosów"
      other: "{count} głosów"
  placeholders:
    count: {type: integer}
`}, "pl")
	noErrors(t, res)

	m := mustGet(t, res, "pl", "tuner.reeds")
	if m.Kind != message.KindPlural {
		t.Fatalf("kind = %v", m.Kind)
	}
	if m.Plural.Argument != "count" {
		t.Fatalf("argument = %q", m.Plural.Argument)
	}
	if len(m.Plural.Forms) != 5 {
		t.Fatalf("%d forms, want 5", len(m.Plural.Forms))
	}
	if m.Plural.Forms[message.CatZero] != "brak głosów" {
		t.Fatal("the zero band was lost")
	}
}

// Real sources use both {count} and {n}.
func TestPluralArgumentIsPerMessage(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/tuner.yaml": `
tones:
  plural:
    argument: n
    forms:
      one: "{n} tone"
      other: "{n} tones"
`})
	noErrors(t, res)

	if got := mustGet(t, res, "en", "tuner.tones").Plural.Argument; got != "n" {
		t.Fatalf("argument = %q, want n", got)
	}
}

func TestTextReturnsEveryRenderedString(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/a.yaml": `
p:
  plural:
    argument: count
    forms:
      other: "b"
      one: "a"
`})
	noErrors(t, res)

	got := mustGet(t, res, "en", "a.p").Text()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("text = %v, want band order", got)
	}
}
