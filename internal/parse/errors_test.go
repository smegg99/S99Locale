// internal/parse/errors_test.go

package parse_test

import (
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
	if e := res.Errors[0]; e.Message != "invalidYAML" || e.Cause == "" {
		t.Fatalf("error = %+v, want invalidYAML with the parser's cause", e)
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
	if res.Errors[0].Message != "bareValue" {
		t.Fatalf("message = %q", res.Errors[0].Message)
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
	if len(res.Errors) != 1 || res.Errors[0].Message != "valueAndPlural" {
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

	if len(res.Errors) != 1 || res.Errors[0].Message != "badPlural" || res.Errors[0].Cause == "" {
		t.Fatalf("errors = %v, want one badPlural with its cause", res.Errors)
	}
}

// Six failures share I18N002, so each has to name its own sentence.
func TestEachSchemaFailureNamesItsMessage(t *testing.T) {
	for want, body := range map[string]string{
		"valueNotString":       "x:\n  value: 3\n",
		"descriptionNotString": "x:\n  value: \"a\"\n  description: 3\n",
		"badPlaceholders":      "x:\n  value: \"a\"\n  placeholders: \"a\"\n",
	} {
		t.Run(want, func(t *testing.T) {
			res := parseOne(t, map[string]string{"en/a.yaml": body})
			if len(res.Errors) != 1 || res.Errors[0].Code != parse.CodeSchema || res.Errors[0].Message != want {
				t.Fatalf("errors = %v, want one %s", res.Errors, want)
			}
		})
	}
}

// An unreadable file is reported with the OS error, and the rest still parses.
func TestAnUnreadableFileIsReported(t *testing.T) {
	dir := root(t, map[string]string{"en/fine.yaml": "ok:\n  value: \"fine\"\n"})
	if err := os.Symlink(filepath.Join(dir, "nowhere.yaml"), filepath.Join(dir, "en", "gone.yaml")); err != nil {
		t.Fatal(err)
	}

	res := roots(t, []string{dir}, []string{"en"})
	if len(res.Errors) != 1 || res.Errors[0].Message != "unreadable" || res.Errors[0].Cause == "" {
		t.Fatalf("errors = %v, want one unreadable with its cause", res.Errors)
	}
	mustGet(t, res, "en", "fine.ok")
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
	if dup.Key != "auth.login.title" || dup.Message != "duplicateKey" {
		t.Fatalf("error = %+v", dup)
	}
	// The message names where the other definition is.
	at, _ := dup.Args["at"].(string)
	if !strings.Contains(at, "auth.yaml") && !strings.Contains(at, "login.yaml") {
		t.Fatalf("args do not point at the other definition: %q", at)
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

	res := roots(t, []string{layer, app}, []string{"en"})
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

	res := roots(t, []string{dir}, []string{"en"})
	if len(res.Errors) != 1 {
		t.Fatalf("errors = %v", res.Errors)
	}

	e := res.Errors[0]
	if e.Message != "fileTooLarge" {
		t.Fatalf("message = %q, want fileTooLarge", e.Message)
	}
	if e.Args["limit"] != parse.MaxFileSize>>20 || e.Args["size"] != parse.MaxFileSize>>20 {
		t.Fatalf("args = %v, want the size and the limit in MB", e.Args)
	}
	if strings.Contains(e.Cause, "token too long") {
		t.Fatalf("the raw scanner error leaked into the cause: %q", e.Cause)
	}
}

// A misspelled field (descripton) would otherwise vanish, and with it the description or placeholders.
func TestAnUnknownMessageFieldIsAnError(t *testing.T) {
	res := parseOne(t, map[string]string{"en/a.yaml": "x:\n  value: \"a\"\n  descripton: \"typo\"\n"})
	if len(res.Errors) != 1 || res.Errors[0].Message != "unknownField" || res.Errors[0].Args["field"] != "descripton" {
		t.Fatalf("errors = %v, want one unknownField naming descripton", res.Errors)
	}
}
