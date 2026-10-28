// internal/validate/validate_test.go

package validate_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/validate"
)

// project writes a config plus locale files and runs parse and validate.
// files is locale-relative: "en/common.yaml" -> contents.
func project(t *testing.T, files map[string]string) *validate.Report {
	t.Helper()
	dir := t.TempDir()

	for name, body := range files {
		full := filepath.Join(dir, "locales", name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	cfgPath := filepath.Join(dir, "config.cue")
	cfgBody := `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web"]
roots: ["locales"]
`
	if err := os.WriteFile(cfgPath, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(context.Background(), cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.Roots(context.Background(), cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}
	return registered(t, validate.Run(cfg, res))
}

// registered fails on a diagnostic whose key the registry lacks.
func registered(t *testing.T, r *validate.Report) *validate.Report {
	t.Helper()
	for _, d := range r.Diagnostics {
		if !slices.Contains(validate.Messages(), d.Message) {
			t.Fatalf("%s %s raised %q, which validate.Messages does not list", d.Code, d.Key, d.Message)
		}
	}
	return r
}

// withMessage is every diagnostic that carries one message key.
func withMessage(r *validate.Report, msg string) []validate.Diagnostic {
	var out []validate.Diagnostic
	for _, d := range r.Diagnostics {
		if d.Message == msg {
			out = append(out, d)
		}
	}
	return out
}

func codes(r *validate.Report) []string {
	var out []string
	for _, d := range r.Diagnostics {
		out = append(out, d.Code)
	}
	return out
}

func has(r *validate.Report, code string) bool {
	return slices.Contains(codes(r), code)
}

func find(t *testing.T, r *validate.Report, code string) validate.Diagnostic {
	t.Helper()
	for _, d := range r.Diagnostics {
		if d.Code == code {
			return d
		}
	}
	t.Fatalf("no %s in %v", code, codes(r))
	return validate.Diagnostic{}
}

const okEn = `
greeting:
  value: "Hello {name}"
  placeholders:
    name: {type: string}
`

const okPl = `
greeting:
  value: "Cześć {name}"
  placeholders:
    name: {type: string}
`

func TestCleanProjectHasNoErrors(t *testing.T) {
	r := project(t, map[string]string{
		"en/common.yaml": okEn,
		"pl/common.yaml": okPl,
	})

	if r.HasErrors() {
		t.Fatalf("errors on a clean project: %v", codes(r))
	}
}

func TestMissingTranslationIsAnError(t *testing.T) {
	r := project(t, map[string]string{
		"en/common.yaml": okEn,
		"pl/common.yaml": "",
	})

	d := find(t, r, validate.CodeMissingKey)
	if d.Locale != "pl" || d.Key != "common.greeting" {
		t.Fatalf("diagnostic = %+v", d)
	}
	if d.Message != "missingKey" {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestKeyMissingFromTheDefaultNamesALocaleThatHasIt(t *testing.T) {
	r := project(t, map[string]string{
		"en/common.yaml": "",
		"pl/common.yaml": okPl,
	})

	d := find(t, r, validate.CodeMissingKey)
	if d.Locale != "en" || d.Args["base"] != "pl" {
		t.Fatalf("diagnostic = %+v, want missing from en and found in pl", d)
	}
	if d.Source.File == "" {
		t.Fatalf("diagnostic has no source; want the pl definition")
	}
}

func TestPlaceholderMismatchAcrossLocales(t *testing.T) {
	r := project(t, map[string]string{
		"en/common.yaml": okEn,
		"pl/common.yaml": `
greeting:
  value: "Cześć {imie}"
  placeholders:
    imie: {type: string}
`})

	d := find(t, r, validate.CodePlaceholder)
	if d.Message != "placeholderMismatch" {
		t.Fatalf("message = %q", d.Message)
	}
	// The diagnostic shows both sides.
	if len(d.Context) != 2 || d.Context[0].Value != "{name}" || d.Context[1].Value != "{imie}" {
		t.Fatalf("diagnostic hides one side: %+v", d.Context)
	}
	if !strings.Contains(d.Source.File, "common.yaml") {
		t.Fatalf("no source location: %+v", d.Source)
	}
}

func TestPlaceholderTypeMismatch(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"{n}\"\n  placeholders:\n    n: {type: integer}\n",
		"pl/a.yaml": "x:\n  value: \"{n}\"\n  placeholders:\n    n: {type: string}\n",
	})

	d := find(t, r, validate.CodeTypeMismatch)
	if d.Message != "placeholderTypeMismatch" || d.Args["name"] != "n" {
		t.Fatalf("diagnostic = %+v; it does not name the placeholder", d)
	}
}

func TestUndeclaredPlaceholderIsAnError(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Hello {name}\"\n",
		"pl/a.yaml": "x:\n  value: \"Cześć {name}\"\n",
	})

	d := find(t, r, validate.CodeUndeclared)
	if d.Message != "placeholderUndeclared" || d.Args["name"] != "name" {
		t.Fatalf("diagnostic = %+v; it does not name the placeholder", d)
	}
}

func TestDeclaredButUnusedIsAWarning(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Hello\"\n  placeholders:\n    name: {type: string}\n",
		"pl/a.yaml": "x:\n  value: \"Cześć\"\n  placeholders:\n    name: {type: string}\n",
	})

	if r.HasErrors() {
		t.Fatalf("an unused placeholder must not fail the build: %v", codes(r))
	}
	d := find(t, r, validate.CodeUnused)
	if d.Message != "placeholderUnused" || d.Args["name"] != "name" {
		t.Fatalf("diagnostic = %+v; it does not name the placeholder", d)
	}
}

func TestEmptyTranslationIsAnError(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Hello\"\n",
		"pl/a.yaml": "x:\n  value: \"   \"\n",
	})

	if d := find(t, r, validate.CodeEmpty); d.Message != "emptyTranslation" {
		t.Fatalf("message = %q", d.Message)
	}
}

func TestKindMismatchAcrossLocales(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"one\"\n",
		"pl/a.yaml": `
x:
  plural:
    argument: count
    forms: {one: "a", few: "b", many: "c", other: "d"}
  placeholders:
    count: {type: integer}
`})

	if d := find(t, r, validate.CodeKindMismatch); d.Message != "kindMismatch" {
		t.Fatalf("message = %q", d.Message)
	}
}

// Run forwards a parse failure whole: its key, arguments and the parser's own text.
func TestAParseFailureKeepsItsMessageArgsAndCause(t *testing.T) {
	r := project(t, map[string]string{
		"en/broken.yaml":     "this: [is: not: yaml\n",
		"en/auth.yaml":       "login:\n  title:\n    value: \"from the group\"\n",
		"en/auth/login.yaml": "title:\n  value: \"from the file\"\n",
	})

	yaml := find(t, r, validate.CodeYAML)
	if yaml.Message != "invalidYAML" || yaml.Cause == "" {
		t.Fatalf("diagnostic = %+v, want invalidYAML with the parser's cause", yaml)
	}

	dup := find(t, r, validate.CodeDuplicate)
	at, _ := dup.Args["at"].(string)
	if dup.Message != "duplicateKey" || !strings.Contains(dup.Source.File, "login.yaml") || !strings.Contains(at, "auth.yaml") {
		t.Fatalf("diagnostic = %+v, want duplicateKey at login.yaml naming auth.yaml", dup)
	}
}

// Only a hand-built set reaches these sites; config and parse reject them earlier.
func TestSitesBehindConfigAndParseNameTheirMessage(t *testing.T) {
	set := message.NewSet([]string{"en", "xx"})
	set.Messages["en"]["a.files"] = &message.Message{
		Key: "a.files", Locale: "en", Kind: message.KindPlural,
		Plural: &message.Plural{Forms: map[message.Category]string{message.CatOne: "a", message.CatOther: "b"}},
	}
	set.Messages["xx"]["a.files"] = &message.Message{Key: "a.files", Locale: "xx", Kind: message.KindPlain, Value: "c"}

	cfg := &config.Config{DefaultLocale: "en", Locales: []string{"en", "xx"}}
	r := registered(t, validate.Run(cfg, &parse.Result{Set: set}))

	for _, want := range []struct{ code, message, locale string }{
		{validate.CodePluralArgument, "pluralArgumentMissing", "en"},
		{validate.CodePluralMissing, "noPluralRule", "xx"},
	} {
		got := withMessage(r, want.message)
		if len(got) != 1 || got[0].Code != want.code || got[0].Locale != want.locale {
			t.Errorf("%s = %+v, want one %s in %s", want.message, got, want.code, want.locale)
		}
	}
	if d := withMessage(r, "noPluralRule"); len(d) == 1 && d[0].Cause == "" {
		t.Errorf("noPluralRule lost plural.For's cause: %+v", d[0])
	}
}

// go-i18n parses message text as a Go template, so doubled braces fail at runtime and render the key.
func TestTemplateBracesAreAnError(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Use {{name}} here\"\n",
		"pl/a.yaml": "x:\n  value: \"Tutaj }}\"\n",
	})

	d := find(t, r, validate.CodeTemplate)
	if d.Message != "templateBraces" {
		t.Fatalf("message = %q", d.Message)
	}
	if n := len(slices.DeleteFunc(codes(r), func(c string) bool { return c != validate.CodeTemplate })); n != 2 {
		t.Fatalf("%d template diagnostics, want one per locale", n)
	}
}
