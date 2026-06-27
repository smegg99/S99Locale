// internal/parse/errors_test.go

package parse_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/parse"
)

func TestInvalidYAMLIsReportedNotFatal(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/broken.yaml": "this: [is: not: yaml\n",
		"en/fine.yaml":   "ok:\n  value: \"fine\"\n",
	})

	if len(res.Errors) != 1 || res.Errors[0].Code != "I18N001" {
		t.Fatalf("errors = %v, want one I18N001", res.Errors)
	}
	// The good file still parsed: one bad file must not hide the rest.
	mustGet(t, res, "en", "fine.ok")
}

func TestBareValueIsRejectedWithAdvice(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/common.yaml": "cancel: \"Cancel\"\n",
	})

	if len(res.Errors) != 1 {
		t.Fatalf("errors = %v, want one", res.Errors)
	}
	if !strings.Contains(res.Errors[0].Detail, "value:") {
		t.Fatalf("detail gives no fix: %q", res.Errors[0].Detail)
	}
}

func TestValueAndPluralTogetherIsRejected(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/a.yaml": `
both:
  value: "x"
  plural:
    argument: count
    forms: {other: "y"}
`})
	if len(res.Errors) != 1 || !strings.Contains(res.Errors[0].Detail, "never both") {
		t.Fatalf("errors = %v", res.Errors)
	}
}

func TestUnknownCategoryIsRejected(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/a.yaml": `
bad:
  plural:
    argument: count
    forms: {several: "x"}
`})

	if len(res.Errors) != 1 {
		t.Fatalf("errors = %v, want one", res.Errors)
	}
}

// A leaf name repeated under several sections resolves to its own line.
func TestARepeatedLeafNameResolvesToItsOwnLine(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/a.yaml": "login:\n  title:\n    value: \"x\"\n" +
			"signup:\n  title:\n    value: \"y\"\n" +
			"settings:\n  title:\n    value: \"z\"\n",
	})

	for _, e := range res.Errors {
		t.Fatalf("fixture does not parse: %v", e)
	}

	for key, want := range map[string]int{
		"a.login.title":    2,
		"a.signup.title":   5,
		"a.settings.title": 8,
	} {
		m, ok := res.Set.Get("en", key)
		if !ok {
			t.Fatalf("no %s", key)
		}
		if m.Source.Line != want {
			t.Errorf("%s line = %d, want %d", key, m.Source.Line, want)
		}
	}
}

func TestDiagnosticsCarryLineAndColumn(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/a.yaml": "# a comment\n\nfirst:\n  value: \"x\"\nbroken: \"bare\"\n",
	})

	if len(res.Errors) != 1 {
		t.Fatalf("errors = %v", res.Errors)
	}
	src := res.Errors[0].Source
	if src.Line != 5 {
		t.Fatalf("line = %d, want 5", src.Line)
	}
	if src.Col != 1 {
		t.Fatalf("col = %d, want 1", src.Col)
	}
}

// A nested group and a nested file reaching the same key is an error, not a walk-order choice.
func TestTwoFilesReachingOneKeyIsAnError(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/auth.yaml":       "login:\n  title:\n    value: \"from the group\"\n",
		"en/auth/login.yaml": "title:\n  value: \"from the file\"\n",
	})

	var dup *parse.Error
	for i, e := range res.Errors {
		if e.Code == "I18N004" {
			dup = &res.Errors[i]
		}
	}
	if dup == nil {
		t.Fatalf("no duplicate reported; errors = %v, keys = %v", res.Errors, res.Set.Keys())
	}
	if dup.Key != "auth.login.title" {
		t.Fatalf("key = %q", dup.Key)
	}
	// The message names where the other definition is.
	if !strings.Contains(dup.Detail, "auth.yaml") && !strings.Contains(dup.Detail, "login.yaml") {
		t.Fatalf("detail does not point at the other definition: %q", dup.Detail)
	}
}

// Later roots win, and every shadowed key is reported.
func TestLaterRootOverridesAndIsReported(t *testing.T) {
	layer := root(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\nclose:\n  value: \"Close\"\n",
	})
	app := root(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Dismiss\"\n",
	})

	res, err := parse.Roots(context.Background(), []string{layer, app}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	noErrors(t, res)

	if got := mustGet(t, res, "en", "common.cancel").Value; got != "Dismiss" {
		t.Fatalf("value = %q, want the app's override", got)
	}
	if got := mustGet(t, res, "en", "common.close").Value; got != "Close" {
		t.Fatalf("value = %q, want the layer's untouched key", got)
	}
	if len(res.Overrides) != 1 || res.Overrides[0].Key != "common.cancel" {
		t.Fatalf("overrides = %v, want common.cancel reported", res.Overrides)
	}
}

func TestMissingLocaleDirectoryIsNotFatal(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/common.yaml": "ok:\n  value: \"x\"\n",
	}, "en", "pl")
	noErrors(t, res)

	if len(res.Set.Messages["pl"]) != 0 {
		t.Fatal("pl came from nowhere")
	}
}

func TestKeysAreSorted(t *testing.T) {
	res := parseOne(t, map[string]string{
		"en/z.yaml": "b:\n  value: \"1\"\na:\n  value: \"2\"\n",
	})
	noErrors(t, res)

	keys := res.Set.Keys()
	if len(keys) != 2 || keys[0] != "z.a" || keys[1] != "z.b" {
		t.Fatalf("keys = %v, want sorted", keys)
	}
}

// An oversized file is rejected before scanning, with the file named.
func TestAnOversizedFileIsReportedNotCryptic(t *testing.T) {
	dir := t.TempDir()
	locale := filepath.Join(dir, "en")

	if err := os.MkdirAll(locale, 0o755); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, parse.MaxFileSize+1)
	for i := range big {
		big[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(locale, "a.yaml"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := parse.Roots(context.Background(), []string{dir}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("errors = %v", res.Errors)
	}

	detail := res.Errors[0].Detail
	if !strings.Contains(detail, "the limit is") || !strings.Contains(detail, "Split it") {
		t.Fatalf("detail = %q, want the size and what to do", detail)
	}
	if strings.Contains(detail, "token too long") {
		t.Fatalf("still the raw scanner error: %q", detail)
	}
}
