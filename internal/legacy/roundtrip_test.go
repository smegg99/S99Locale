// internal/legacy/roundtrip_test.go

package legacy_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/legacy"
	"github.com/smegg99/s99locale/internal/parse"
)

// The importer's strongest test: `loc build` emits exactly the two formats
// `loc import` reads, so build -> import -> build must land on identical bytes.
//
// Anything the importer loses shows up as a diff. This runs against the same
// content S99AB and S99ABDemo carry, without depending on either repo.
const canonicalCfg = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]
roots: ["locales"]
namespaces: {
	common: {targets: ["web"]}
	report: {targets: ["backend"]}
}
go: {package: "messages"}
`

var canonical = map[string]string{
	// The layer's vocabulary, as S99AB ships it.
	"en/common.yaml": `
cancel:
  value: "Cancel"
confirm:
  value: "Confirm"
retry:
  value: "Retry"
greeting:
  value: "Hello {name}"
  placeholders:
    name: {type: string}
items:
  plural:
    argument: count
    forms:
      zero: "No items yet"
      one: "{count} item"
      other: "{count} items"
  placeholders:
    count: {type: integer}
`,
	"pl/common.yaml": `
cancel:
  value: "Anuluj"
confirm:
  value: "Zatwierdź"
retry:
  value: "Ponów"
greeting:
  value: "Cześć {name}"
  placeholders:
    name: {type: string}
items:
  plural:
    argument: count
    forms:
      zero: "Brak elementów"
      one: "{count} element"
      few: "{count} elementy"
      many: "{count} elementów"
      other: "{count} elementu"
  placeholders:
    count: {type: integer}
`,
	// Backend only, so it exercises the flat go-i18n path.
	"en/report.yaml": `
heading:
  value: "Item report"
summary:
  value: "Generated for {owner}"
  placeholders:
    owner: {type: string}
`,
	"pl/report.yaml": `
heading:
  value: "Raport elementów"
summary:
  value: "Wygenerowano dla {owner}"
  placeholders:
    owner: {type: string}
`,
}

func project(t *testing.T, files map[string]string) (*config.Config, string) {
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

	path := filepath.Join(dir, "config.cue")
	if err := os.WriteFile(path, []byte(canonicalCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, dir
}

func build(t *testing.T, cfg *config.Config) *generate.Output {
	t.Helper()

	res, err := parse.Roots(context.Background(), cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Errors {
		t.Fatalf("canonical source does not parse: %v", e)
	}

	out, err := generate.Build(context.Background(), cfg, res.Set)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := out.Write(); err != nil {
		t.Fatal(err)
	}
	return out
}

func bundles(out *generate.Output) map[string][]byte {
	got := map[string][]byte{}
	for _, f := range out.Files() {
		// Only the runtime bundles round-trip. Typed keys and the manifest are
		// derived, not a source of truth an importer could read back.
		if f.Path == "manifest.json" ||
			filepath.Ext(f.Path) == ".ts" ||
			filepath.Ext(f.Path) == ".go" {
			continue
		}
		got[f.Path] = f.Bytes
	}
	return got
}

func TestBuildImportBuildIsStable(t *testing.T) {
	cfg, _ := project(t, canonical)
	first := build(t, cfg)

	// Import what the build just wrote, into a fresh project.
	generated := cfg.AbsOutput()
	res, err := legacy.Import(legacy.Sources{
		Go:  filepath.Join(generated, "backend"),
		Vue: filepath.Join(generated, "web"),
	}, cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}

	if len(res.Notes) != 0 {
		t.Fatalf("a round trip of loc's own output needed a human: %v", notes(res))
	}

	reimported := map[string]string{}
	for path, body := range res.Files {
		reimported[path] = string(body)
	}

	second, _ := project(t, reimported)
	out := build(t, second)

	want, got := bundles(first), bundles(out)
	// Guard against a vacuous pass: an empty set would satisfy every loop.
	if len(want) < 4 {
		t.Fatalf("only %d bundles to compare; the fixture is not exercising both targets", len(want))
	}

	for path, wantBytes := range want {
		gotBytes, ok := got[path]
		if !ok {
			t.Errorf("%s vanished on the round trip", path)
			continue
		}
		if !slices.Equal(wantBytes, gotBytes) {
			t.Errorf("%s differs after a round trip:\n  before %s\n  after  %s",
				path, wantBytes, gotBytes)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("%s appeared from nowhere", path)
		}
	}
}

// The zero band is the piece most likely to be lost: it leaves as a separate
// go-i18n key and as the first vue band.
func TestZeroBandSurvivesTheRoundTrip(t *testing.T) {
	cfg, _ := project(t, canonical)
	build(t, cfg)

	res, err := legacy.Import(legacy.Sources{
		Vue: filepath.Join(cfg.AbsOutput(), "web"),
	}, cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}

	pl := imported(t, res, "pl/common.yaml")
	for _, want := range []string{`zero: "Brak elementów"`, `few: "{count} elementy"`, `many: "{count} elementów"`} {
		if !strings.Contains(pl, want) {
			t.Fatalf("lost %s:\n%s", want, pl)
		}
	}
}
