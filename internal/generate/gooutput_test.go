// internal/generate/gooutput_test.go

package generate_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
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
go: {package: "messages", output: "../pkg/messages"}
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

	cfg, err := config.Load(filepath.Join(dir, "i18n/config.cue"))
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

// The accessors go beside the package's own source and the bundle goes in locales/, for go:embed.
func TestGoOutputLandsInTheConsumersPackage(t *testing.T) {
	_, out, dir := shared(t)

	if _, _, err := out.Write(); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		"pkg/messages/keys_gen.go",
		"pkg/messages/messages_gen.go",
		"pkg/messages/locales/en.json",
		"pkg/messages/locales/pl.json",
	} {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
	// Nothing is written under the output directory.
	if _, err := os.Stat(filepath.Join(dir, "i18n/generated/go")); !os.IsNotExist(err) {
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
	// A locale an older config produced is loc's, and would otherwise be swept
	// back up by the embed directive.
	if _, err := os.Stat(filepath.Join(dir, "pkg/messages/locales/de.json")); !os.IsNotExist(err) {
		t.Error("a stale bundle survived in the package loc writes to")
	}
}

// check reports a stale file in the second tree by a path relative to the config.
func TestDiffNamesTheSharedTree(t *testing.T) {
	_, out, dir := shared(t)

	stale, err := out.Diff()
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, p := range stale {
		if filepath.Base(p) == "keys_gen.go" && filepath.IsAbs(p) {
			found = true
		}
	}
	if !found {
		t.Fatalf("stale = %v, want an absolute path to keys_gen.go under %s", stale, dir)
	}
}

func TestOwnsOnlyClaimsGeneratedPaths(t *testing.T) {
	for path, want := range map[string]bool{
		"keys_gen.go":       true,
		"messages_gen.go":   true,
		"locales/pl.json":   true,
		"messages.go":       false,
		"doc.go":            false,
		"internal/thing.go": false,
		"other/keys_gen.go": false,
	} {
		if got := generate.Owns(path); got != want {
			t.Errorf("Owns(%q) = %v, want %v", path, got, want)
		}
	}
}
