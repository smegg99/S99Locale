// internal/generate/gooutput_test.go

package generate_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
)

const sharedCfg = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]
roots: ["locales"]
output: "generated"
namespaces: {
	common: {targets: ["web"]}
	tray:   {targets: ["backend"]}
}
outputs: {backend: "../pkg/messages"}
go: {package: "messages"}
`

// shared builds a project whose Go target shares a package with hand-written source.
func shared(t *testing.T) (*config.Config, *generate.Outputs, string) {
	t.Helper()
	dir := t.TempDir()

	write := func(rel, body string) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("i18n/locales/en/common.yaml", "greeting:\n  value: \"Hello\"\n  description: \"d\"\n")
	write("i18n/locales/pl/common.yaml", "greeting:\n  value: \"Witaj\"\n  description: \"d\"\n")
	write("i18n/locales/en/tray.yaml", "open:\n  value: \"Open\"\n  description: \"d\"\n")
	write("i18n/locales/pl/tray.yaml", "open:\n  value: \"Otworz\"\n  description: \"d\"\n")
	write("i18n/config.cue", sharedCfg)
	// The package's own source, and a stale locale file.
	write("pkg/messages/messages.go", "package messages\n")
	write("pkg/messages/locales/de.json", "{}\n")

	cfg, err := config.Load(context.Background(), filepath.Join(dir, "i18n/config.cue"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.Roots(context.Background(), cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Errors {
		t.Fatalf("fixture does not parse: %v", e)
	}
	out, err := generate.Build(context.Background(), cfg, res.Set)
	if err != nil {
		t.Fatal(err)
	}
	return cfg, out, dir
}

// rebuild parses and generates again from the same config, like a second `loc build`.
func rebuild(t *testing.T, cfg *config.Config) *generate.Outputs {
	t.Helper()

	res, err := parse.Roots(context.Background(), cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}
	out, err := generate.Build(context.Background(), cfg, res.Set)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// reload rewrites one line of the config and loads it again.
func reload(t *testing.T, cfg *config.Config, from, to string) *config.Config {
	t.Helper()

	path := filepath.Join(cfg.Dir, "config.cue")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(raw), from, to, 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	next, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

// buildAt builds the shared fixture and returns the backend record's bytes.
// spelling is the config path relative to the fixture root; empty means absolute.
func buildAt(t *testing.T, spelling string) []byte {
	t.Helper()
	_, _, dir := shared(t)

	path := filepath.Join(dir, "i18n", "config.cue")
	if spelling != "" {
		// t.Chdir restores the working directory on cleanup.
		t.Chdir(dir)
		path = spelling
	}
	reloaded, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}

	built := rebuild(t, reloaded)
	for _, f := range built.For(message.TargetBackend).Files() {
		if f.Path == generate.ManifestName {
			return f.Bytes
		}
	}
	t.Fatal("no record in the backend tree")
	return nil
}

// The accessors go beside the package's own source and the bundle goes in locales/, for go:embed.
func TestGoOutputLandsInTheConsumersPackage(t *testing.T) {
	_, out, dir := shared(t)

	if _, _, err := out.Write(); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"pkg/messages/keys_gen.go",
		"pkg/messages/messages_gen.go",
		"pkg/messages/loc-manifest.json",
		"pkg/messages/locales/en.json",
		"pkg/messages/locales/pl.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
	// Nothing is written under the output directory.
	if _, err := os.Stat(filepath.Join(dir, "i18n/generated/backend")); !os.IsNotExist(err) {
		t.Error("the Go target was written under output as well")
	}
}

// loc owns its own files in that package and nothing else in it.
func TestASharedPackageKeepsItsOwnSource(t *testing.T) {
	_, out, dir := shared(t)

	if _, _, err := out.Write(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pkg/messages/messages.go")); err != nil {
		t.Errorf("hand-written source was removed: %v", err)
	}
}

// check reports a stale file in the second tree by a path relative to the config.
func TestDiffNamesTheSharedTree(t *testing.T) {
	_, outs, _ := shared(t)

	stale, _, err := outs.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(stale, "../pkg/messages/keys_gen.go") {
		t.Fatalf("stale = %v, want ../pkg/messages/keys_gen.go", stale)
	}
}

// Another generator's output in the shared package is not loc's to delete.
func TestAForeignGeneratedFileSurvivesABuild(t *testing.T) {
	_, out, dir := shared(t)

	foreign := filepath.Join(dir, "pkg/messages/cue_types_thing_gen.go")
	body := "// Code generated by cue exp gengotypes. DO NOT EDIT.\n\npackage messages\n"
	if err := os.WriteFile(foreign, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	_, removed, err := out.Write()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("loc deleted another generator's output: %v", err)
	}
	for _, name := range removed {
		if filepath.Base(name) == "cue_types_thing_gen.go" {
			t.Fatal("loc reported removing a file it does not own")
		}
	}
	// A first build into that package has no record, so it removes nothing at all.
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want nothing", removed)
	}
}

// The README promises loc never overwrites a file it did not write, and a first build has no record yet.
func TestAFirstBuildRefusesToOverwriteAHandWrittenFile(t *testing.T) {
	_, out, dir := shared(t)

	mine := filepath.Join(dir, "pkg/messages/keys_gen.go")
	body := "package messages\n\n// Written by hand before loc ever ran here.\n"
	if err := os.WriteFile(mine, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := out.Write(); err == nil {
		t.Fatal("the first build overwrote a file loc never wrote")
	}
	if got, _ := os.ReadFile(mine); string(got) != body {
		t.Fatal("the hand-written file changed")
	}
}
