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
	"github.com/smegg99/s99locale/internal/message"
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
	if err := json.Unmarshal(file(t, out, "web/loc-manifest.json"), &man); err != nil {
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
	if man["target"] != "web" {
		t.Fatalf("target = %v", man["target"])
	}
	if got := man["namespaces"].([]any); len(got) != 1 || got[0] != "common" {
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

	stale, modified, err := out.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(stale)+len(modified) != 0 {
		t.Fatalf("check reported %v stale and %v modified right after a build", stale, modified)
	}

	// No temp files left behind.
	entries, err := os.ReadDir(filepath.Join(out.For(message.TargetWeb).Dir, "en"))
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

	web := out.For(message.TargetWeb)
	target := filepath.Join(web.Dir, "en", "common.json")
	if err := os.WriteFile(target, []byte(`{"tampered":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	stale, modified, err := out.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(modified) != 0 {
		t.Fatalf("modified = %v; an edited file loc still produces is stale, not modified", modified)
	}
	if !slices.Contains(stale, "generated/web/en/common.json") {
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

	cfg, err := config.Load(context.Background(), cfgPath)
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

	for _, path := range []string{"backend/messages_gen.go", "backend/keys_gen.go"} {
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
	cfg, out := project(t, fixture)
	if _, _, err := out.Write(); err != nil {
		t.Fatal(err)
	}

	// Drop pl, so what the last build wrote for it is an orphan its record names.
	narrowed := reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en"]`)
	next := rebuild(t, narrowed)
	orphans := []string{"generated/backend/locales/pl.json", "generated/web/pl/common.json"}

	stale, modified, err := next.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(modified) != 0 {
		t.Fatalf("modified = %v, want nothing: nobody edited a file", modified)
	}
	for _, want := range orphans {
		if !slices.Contains(stale, want) {
			t.Fatalf("check did not report the orphan %s: %v", want, stale)
		}
	}

	_, removed, err := next.Write()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(removed, orphans) {
		t.Fatalf("removed = %v, want exactly %v", removed, orphans)
	}
	web := next.For(message.TargetWeb).Dir
	if _, err := os.Stat(filepath.Join(web, "pl", "common.json")); !os.IsNotExist(err) {
		t.Fatalf("orphan survived the build: %v", err)
	}
	// The directory goes too, or a glob still matches an empty locale.
	if _, err := os.Stat(filepath.Join(web, "pl")); !os.IsNotExist(err) {
		t.Fatalf("empty directory survived the build: %v", err)
	}

	// Everything the build does produce is still there.
	stale, modified, err = next.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if len(stale)+len(modified) != 0 {
		t.Fatalf("build left %v stale and %v modified", stale, modified)
	}
}
