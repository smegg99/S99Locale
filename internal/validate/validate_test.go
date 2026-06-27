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

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.Roots(context.Background(), cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}
	return validate.Run(cfg, res)
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
	// The diagnostic shows both sides.
	rendered := d.String()
	if !strings.Contains(rendered, "{name}") || !strings.Contains(rendered, "{imie}") {
		t.Fatalf("diagnostic hides one side:\n%s", rendered)
	}
	if !strings.Contains(rendered, "common.yaml") {
		t.Fatalf("no source location:\n%s", rendered)
	}
}

func TestPlaceholderTypeMismatch(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"{n}\"\n  placeholders:\n    n: {type: integer}\n",
		"pl/a.yaml": "x:\n  value: \"{n}\"\n  placeholders:\n    n: {type: string}\n",
	})

	if !has(r, validate.CodeTypeMismatch) {
		t.Fatalf("codes = %v", codes(r))
	}
}

func TestUndeclaredPlaceholderIsAnError(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Hello {name}\"\n",
		"pl/a.yaml": "x:\n  value: \"Cześć {name}\"\n",
	})

	if !has(r, validate.CodeUndeclared) {
		t.Fatalf("codes = %v", codes(r))
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
	if !has(r, validate.CodeUnused) {
		t.Fatalf("codes = %v", codes(r))
	}
}

func TestEmptyTranslationIsAnError(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Hello\"\n",
		"pl/a.yaml": "x:\n  value: \"   \"\n",
	})

	if !has(r, validate.CodeEmpty) {
		t.Fatalf("codes = %v", codes(r))
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

	if !has(r, validate.CodeKindMismatch) {
		t.Fatalf("codes = %v", codes(r))
	}
}
