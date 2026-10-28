// internal/generate/clean_test.go

package generate_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
)

// Clean removes recorded files, records and emptied directories, and nothing loc did not write.
func TestCleanRemovesEverythingTheRecordsName(t *testing.T) {
	cfg, outs, dir := shared(t)
	if _, _, err := outs.Write(); err != nil {
		t.Fatal(err)
	}

	removed, err := generate.Clean(cfg)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"../pkg/messages/embed_gen.go",
		"../pkg/messages/keys_gen.go",
		"../pkg/messages/loc-manifest.json",
		"../pkg/messages/locales/en.json",
		"../pkg/messages/locales/pl.json",
		"../pkg/messages/messages_gen.go",
		"generated/web/en/common.json",
		"generated/web/keys.gen.ts",
		"generated/web/loc-manifest.json",
		"generated/web/pl/common.json",
		"generated/web/plurals.gen.ts",
	}
	if !slices.Equal(removed, want) {
		t.Fatalf("removed = %v, want %v", removed, want)
	}

	// The web tree and the default parent held nothing else, so they go with it.
	if _, err := os.Stat(filepath.Join(dir, "i18n", "generated")); !os.IsNotExist(err) {
		t.Fatalf("an emptied directory was left behind: %v", err)
	}
	// The package's own source and the hand-written locale stay.
	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	for _, rel := range []string{"messages.go", "locales/de.json"} {
		if _, err := os.Stat(filepath.Join(pkg, rel)); err != nil {
			t.Errorf("%s was removed: %v", rel, err)
		}
	}

	removed, err = generate.Clean(cfg)
	if err != nil || len(removed) != 0 {
		t.Fatalf("second clean: removed %v, err %v; want nothing", removed, err)
	}
}

// Without a record nothing is provably loc's, so clean removes nothing.
func TestCleanWithNoRecordRemovesNothing(t *testing.T) {
	cfg, _, dir := shared(t)
	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	writeFile(t, filepath.Join(pkg, "keys_gen.go"), []byte("package messages\n"))

	removed, err := generate.Clean(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want nothing without a record", removed)
	}
	for _, rel := range []string{"keys_gen.go", "messages.go", "locales/de.json"} {
		if _, err := os.Stat(filepath.Join(pkg, rel)); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "i18n", "generated")); !os.IsNotExist(err) {
		t.Fatalf("clean created the output tree: %v", err)
	}
}

// An edited file stops clean before it removes anything, in that tree or any other.
func TestCleanRefusesAnEditedFile(t *testing.T) {
	cfg, outs, dir := shared(t)
	if _, _, err := outs.Write(); err != nil {
		t.Fatal(err)
	}

	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	victim := filepath.Join(pkg, "locales", "pl.json")
	writeFile(t, victim, []byte(`{"mine":"now"}`))

	removed, err := generate.Clean(cfg)
	if err == nil || !strings.Contains(err.Error(), "../pkg/messages/locales/pl.json was modified since loc wrote it; remove it by hand") {
		t.Fatalf("err = %v, want a refusal naming the edited file", err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed %v before refusing", removed)
	}
	for _, keep := range []string{
		victim,
		filepath.Join(pkg, generate.ManifestName),
		filepath.Join(dir, "i18n", "generated", "web", "pl", "common.json"),
	} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s: %v", keep, err)
		}
	}
}

// A dropped target's default tree is cleared by its record, as the next build would clear it.
func TestCleanClearsARetiredTree(t *testing.T) {
	cfg, _, dir := shared(t)
	both := configure(t, cfg, bothAtDefault)
	if _, _, err := rebuild(t, both).Write(); err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(dir, "i18n", "generated", "backend")
	notes := filepath.Join(backend, "NOTES.txt")
	writeFile(t, notes, []byte("mine\n"))

	removed, err := generate.Clean(configure(t, both, webAtDefault))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"generated/backend/keys_gen.go",
		"generated/backend/loc-manifest.json",
		"generated/backend/locales/en.json",
		"generated/backend/locales/pl.json",
		"generated/backend/messages_gen.go",
		"generated/web/loc-manifest.json",
	} {
		if !slices.Contains(removed, want) {
			t.Errorf("removed = %v, want %s", removed, want)
		}
	}
	if _, err := os.Stat(notes); err != nil {
		t.Fatal("a file loc never wrote went with the retired tree")
	}
	if _, err := os.Stat(filepath.Join(backend, "locales")); !os.IsNotExist(err) {
		t.Fatalf("the emptied locales directory survived: %v", err)
	}
}

// A tree that was never built has no record, and clean creates nothing looking for one.
func TestCleanOfATreeNeverBuiltCreatesNothing(t *testing.T) {
	cfg, _, dir := shared(t)

	removed, err := generate.Clean(configure(t, cfg, webAtDefault))
	if err != nil || len(removed) != 0 {
		t.Fatalf("removed %v, err %v; want nothing", removed, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "i18n", "generated")); !os.IsNotExist(err) {
		t.Fatalf("clean created the output tree: %v", err)
	}
}
