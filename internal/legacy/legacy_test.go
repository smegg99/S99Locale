// internal/legacy/legacy_test.go

package legacy_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/legacy"
)

func tree(t *testing.T, files map[string]string) string {
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

func imported(t *testing.T, res *legacy.Result, path string) string {
	t.Helper()
	body, ok := res.Files[path]
	if !ok {
		var have []string
		for p := range res.Files {
			have = append(have, p)
		}
		t.Fatalf("no %s; have %v", path, have)
	}
	return string(body)
}

func notes(res *legacy.Result) []string {
	var out []string
	for _, n := range res.Notes {
		out = append(out, n.Key+": "+n.Detail)
	}
	return out
}

// go-i18n's {{.Name}} has to come back as canonical {name}.
func TestGoTemplateSyntaxIsUndone(t *testing.T) {
	dir := tree(t, map[string]string{
		"en.json": `{"emails.welcome": "Hello {{.Name}}, welcome to {{.Product}}"}`,
	})

	res, err := legacy.Import(legacy.Sources{Go: dir}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}

	body := imported(t, res, "en/emails.yaml")
	if !strings.Contains(body, "{name}") || !strings.Contains(body, "{product}") {
		t.Fatalf("template syntax survived:\n%s", body)
	}
	if strings.Contains(body, "{{") {
		t.Fatalf("Go syntax reached a canonical file:\n%s", body)
	}
	// Placeholders are declared, or validation would reject the result.
	if !strings.Contains(body, "name: {type: string}") {
		t.Fatalf("placeholders not inferred:\n%s", body)
	}
}

// The whole point: four Polish bands recovered from a pipe string.
func TestPolishPipeStringBecomesFourBands(t *testing.T) {
	dir := tree(t, map[string]string{
		"pl/tuner.json": `{"tuner":{"reeds":"{count} głos | {count} głosy | {count} głosów | {count} głosu"}}`,
	})

	res, err := legacy.Import(legacy.Sources{Vue: dir}, []string{"pl"})
	if err != nil {
		t.Fatal(err)
	}

	body := imported(t, res, "pl/tuner.yaml")
	for _, cat := range []string{"one:", "few:", "many:", "other:"} {
		if !strings.Contains(body, cat) {
			t.Fatalf("missing %s:\n%s", cat, body)
		}
	}
	if !strings.Contains(body, "argument: count") {
		t.Fatalf("count argument not inferred:\n%s", body)
	}
	if len(res.Notes) != 0 {
		t.Fatalf("an unambiguous import reported %v", notes(res))
	}
}

// Five bands means the first is the opt-in zero.
func TestFiveBandsRecoversTheZeroForm(t *testing.T) {
	dir := tree(t, map[string]string{
		"pl/tuner.json": `{"tuner":{"reeds":"brak głosów | {count} głos | {count} głosy | {count} głosów | {count} głosu"}}`,
	})

	res, err := legacy.Import(legacy.Sources{Vue: dir}, []string{"pl"})
	if err != nil {
		t.Fatal(err)
	}

	body := imported(t, res, "pl/tuner.yaml")
	if !strings.Contains(body, `zero: "brak głosów"`) {
		t.Fatalf("zero band not recovered:\n%s", body)
	}
}

// SmeggTuner's real shape: a message with fewer bands than Polish needs. The
// clamping made it render, but a band is genuinely missing and no tool can
// invent the wording.
func TestTooFewBandsIsReportedNotGuessed(t *testing.T) {
	dir := tree(t, map[string]string{
		"pl/a.json": `{"a":{"files":"{count} plik | {count} plików"}}`,
	})

	res, err := legacy.Import(legacy.Sources{Vue: dir}, []string{"pl"})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Notes) == 0 {
		t.Fatal("a two-band Polish plural imported silently")
	}
	if !strings.Contains(res.Notes[0].Detail, "needs 4") {
		t.Fatalf("note does not say what is missing: %v", notes(res))
	}
}

// A plural carrying more than one placeholder cannot be resolved by counting.
func TestAmbiguousCountArgumentIsReported(t *testing.T) {
	dir := tree(t, map[string]string{
		"en/a.json": `{"a":{"x":"{banks} banks, {registers} register | {banks} banks, {registers} registers"}}`,
	})

	res, err := legacy.Import(legacy.Sources{Vue: dir}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Notes) == 0 {
		t.Fatal("an ambiguous count argument was chosen silently")
	}
}

// A single placeholder across every band is unambiguous whatever it is called.
func TestNonStandardCountArgument(t *testing.T) {
	dir := tree(t, map[string]string{
		"en/a.json": `{"a":{"x":"{n} tone | {n} tones"}}`,
	})

	res, err := legacy.Import(legacy.Sources{Vue: dir}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}

	body := imported(t, res, "en/a.yaml")
	if !strings.Contains(body, "argument: n") {
		t.Fatalf("argument not inferred:\n%s", body)
	}
	if !strings.Contains(body, "n: {type: integer}") {
		t.Fatalf("the count should be an integer:\n%s", body)
	}
	if len(res.Notes) != 0 {
		t.Fatalf("unexpected notes: %v", notes(res))
	}
}

func TestNestedVueKeysNest(t *testing.T) {
	dir := tree(t, map[string]string{
		"en/common.json": `{"common":{"action":{"accept":"OK","cancel":"Cancel"}}}`,
	})

	res, err := legacy.Import(legacy.Sources{Vue: dir}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}

	body := imported(t, res, "en/common.yaml")
	// The namespace prefix is the file name, so it must not repeat inside.
	if strings.Contains(body, "common:") {
		t.Fatalf("namespace repeated inside its own file:\n%s", body)
	}
	if !strings.Contains(body, "action:") || !strings.Contains(body, "accept:") {
		t.Fatalf("structure lost:\n%s", body)
	}
}

// A bundle not wrapped in its namespace is also common in the wild.
func TestUnwrappedVueBundleGetsItsNamespace(t *testing.T) {
	dir := tree(t, map[string]string{
		"en/common.json": `{"cancel":"Cancel"}`,
	})

	res, err := legacy.Import(legacy.Sources{Vue: dir}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}

	body := imported(t, res, "en/common.yaml")
	if !strings.Contains(body, "cancel:") {
		t.Fatalf("key lost:\n%s", body)
	}
}

// The two trees disagreeing is the drift being migrated away from.
func TestConflictBetweenTreesIsReported(t *testing.T) {
	goDir := tree(t, map[string]string{"en.json": `{"common.cancel": "Cancel"}`})
	vueDir := tree(t, map[string]string{"en/common.json": `{"common":{"cancel":"Dismiss"}}`})

	res, err := legacy.Import(legacy.Sources{Go: goDir, Vue: vueDir}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Notes) == 0 {
		t.Fatal("two trees disagreeing imported silently")
	}
	if !strings.Contains(res.Notes[0].Detail, "disagree") {
		t.Fatalf("notes = %v", notes(res))
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	files := map[string]string{
		"en/common.json": `{"common":{"b":"B","a":"A","c":{"d":"D"}}}`,
	}

	first, err := legacy.Import(legacy.Sources{Vue: tree(t, files)}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := legacy.Import(legacy.Sources{Vue: tree(t, files)}, []string{"en"})
	if err != nil {
		t.Fatal(err)
	}

	if imported(t, first, "en/common.yaml") != imported(t, second, "en/common.yaml") {
		t.Fatal("two imports of the same input differ")
	}
}

func TestMissingLocaleIsNotFatal(t *testing.T) {
	dir := tree(t, map[string]string{"en/a.json": `{"a":{"x":"X"}}`})

	res, err := legacy.Import(legacy.Sources{Vue: dir}, []string{"en", "pl"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.Files["pl/a.yaml"]; ok {
		t.Fatal("pl came from nowhere")
	}
}
