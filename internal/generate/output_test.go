// internal/generate/output_test.go

package generate_test

import (
	"context"
	"encoding/json"
	"go/format"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/parse"
)

// Two builds on the same input must produce identical bytes.
func TestDeterministic(t *testing.T) {
	_, first := project(t, fixture)
	_, second := project(t, fixture)

	a, b := first.Files(), second.Files()
	if len(a) != len(b) {
		t.Fatalf("%d files vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Path != b[i].Path {
			t.Fatalf("path %d: %s vs %s", i, a[i].Path, b[i].Path)
		}
		if !slices.Equal(a[i].Bytes, b[i].Bytes) {
			t.Fatalf("%s differs between builds", a[i].Path)
		}
	}
}

// The manifest must not carry a timestamp, or every build differs.
func TestManifestHasNoTimestamp(t *testing.T) {
	_, out := project(t, fixture)

	var man map[string]any
	if err := json.Unmarshal(file(t, out, "manifest.json"), &man); err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{"generatedAt", "timestamp", "date", "time"} {
		if _, ok := man[banned]; ok {
			t.Fatalf("manifest carries %q", banned)
		}
	}
	if man["sourceHash"] == "" {
		t.Fatal("no source hash")
	}

	targets := man["targets"].(map[string]any)
	web := targets["web"].(map[string]any)
	if got := web["namespaces"].([]any); len(got) != 1 || got[0] != "common" {
		t.Fatalf("web namespaces = %v", got)
	}
}

func TestWriteIsAtomicAndSkipsUnchanged(t *testing.T) {
	cfg, out := project(t, fixture)
	_ = cfg

	written, _, err := out.Write()
	if err != nil {
		t.Fatal(err)
	}
	if written != len(out.Files()) {
		t.Fatalf("wrote %d of %d on a clean directory", written, len(out.Files()))
	}

	// Nothing changed, so nothing should be rewritten.
	again, _, err := out.Write()
	if err != nil {
		t.Fatal(err)
	}
	if again != 0 {
		t.Fatalf("rewrote %d unchanged file(s)", again)
	}

	stale, err := out.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Fatalf("check reported %v stale right after a build", stale)
	}

	// No temp files left behind.
	entries, err := os.ReadDir(filepath.Join(out.Dir, "web", "en"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".loc-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestDiffDetectsAnEditedFile(t *testing.T) {
	_, out := project(t, fixture)
	if _, _, err := out.Write(); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(out.Dir, "web", "en", "common.json")
	if err := os.WriteFile(target, []byte(`{"tampered":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stale, err := out.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(stale, "web/en/common.json") {
		t.Fatalf("stale = %v", stale)
	}
}

// A literal pipe would silently split a message into another plural band.
func TestPipeInAPluralFormIsRejected(t *testing.T) {
	dir := t.TempDir()
	body := `
x:
  plural:
    argument: count
    forms: {one: "a | b", other: "c"}
  placeholders:
    count: {type: integer}
`
	for _, locale := range []string{"en", "pl"} {
		full := filepath.Join(dir, "locales", locale, "common.yaml")
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfgPath := filepath.Join(dir, "config.cue")
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
	if _, err := generate.Build(context.Background(), cfg, res.Set); err == nil {
		t.Fatal("a literal pipe was accepted into a vue plural")
	}
}

// Generated Go must already be gofmt-canonical.
func TestGeneratedGoIsGofmtStable(t *testing.T) {
	out := pluralProject(t)

	for _, path := range []string{"go/messages_gen.go", "go/keys_gen.go"} {
		src := file(t, out, path)
		formatted, err := format.Source(src)
		if err != nil {
			t.Fatalf("%s is not valid Go: %v", path, err)
		}
		if !slices.Equal(src, formatted) {
			t.Fatalf("%s is not gofmt-canonical as emitted", path)
		}
	}
}

// Dropping a locale removes its recorded files and directory.
func TestWriteRemovesOutputItNoLongerProduces(t *testing.T) {
	_, out := project(t, fixture)
	if _, _, err := out.Write(); err != nil {
		t.Fatal(err)
	}

	orphanDir := filepath.Join(out.Dir, "web", "de")
	if err := os.MkdirAll(orphanDir, 0o755); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(orphanDir, "common.json")
	if err := os.WriteFile(orphan, []byte(`{"common":{"ok":"[Ök··]"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stale, err := out.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 1 || stale[0] != filepath.Join("web", "de", "common.json") {
		t.Fatalf("check did not report the orphan: %v", stale)
	}

	_, removed, err := out.Write()
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed %d orphan(s), want 1", removed)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("orphan survived the build: %v", err)
	}
	// The directory goes too, or a glob still matches an empty locale.
	if _, err := os.Stat(orphanDir); !os.IsNotExist(err) {
		t.Fatalf("empty directory survived the build: %v", err)
	}

	// Everything the build does produce is still there.
	stale, err = out.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(stale) != 0 {
		t.Fatalf("build left %v stale", stale)
	}
}
