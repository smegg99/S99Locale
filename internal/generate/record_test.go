// internal/generate/record_test.go

package generate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
)

// A first build into a directory with no record removes nothing.
func TestAFirstBuildIntoAnExistingPackageRemovesNothing(t *testing.T) {
	_, outs, dir := shared(t)

	pkg := filepath.Join(dir, "pkg/messages")
	for _, rel := range []string{"messages.go", "fault.go", "cue_types_thing_gen.go", "locales/de.json"} {
		full := filepath.Join(pkg, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, removed, err := outs.Write()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want nothing on a first build", removed)
	}
	for _, rel := range []string{"messages.go", "fault.go", "cue_types_thing_gen.go", "locales/de.json"} {
		if _, err := os.Stat(filepath.Join(pkg, rel)); err != nil {
			t.Errorf("%s: %v", rel, err)
		}
	}
}

// What the record names, and only that, is removed on the next build.
func TestASecondBuildRemovesWhatTheRecordNames(t *testing.T) {
	cfg, outs, _ := shared(t)
	if _, _, err := outs.Write(); err != nil {
		t.Fatal(err)
	}

	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	if err := os.WriteFile(filepath.Join(pkg, "locales", "de.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The build that follows does not produce de.json and never recorded it.
	second := rebuild(t, cfg)
	_, removed, err := second.Write()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 0 {
		t.Fatalf("removed = %v, want nothing: de.json is in no record", removed)
	}
	if _, err := os.Stat(filepath.Join(pkg, "locales", "de.json")); err != nil {
		t.Fatal("a file loc never wrote was removed")
	}

	// pl is in the record, so dropping it from the config removes its files.
	narrowed := reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en"]`)
	_, removed, err = rebuild(t, narrowed).Write()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"../pkg/messages/locales/pl.json", "generated/web/pl/common.json"}
	if !slices.Equal(removed, want) {
		t.Fatalf("removed = %v, want exactly %v", removed, want)
	}
	if _, err := os.Stat(filepath.Join(pkg, "locales", "de.json")); err != nil {
		t.Fatal("a file loc never wrote was removed with the ones it did")
	}
}

// The record is the proof, and a hand edit destroys it.
func TestAnEditedFileIsNeverRemoved(t *testing.T) {
	cfg, outs, _ := shared(t)
	if _, _, err := outs.Write(); err != nil {
		t.Fatal(err)
	}

	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	victim := filepath.Join(pkg, "locales", "pl.json")
	if err := os.WriteFile(victim, []byte(`{"mine":"now"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Drop pl from the config, so pl.json becomes an orphan the record names.
	narrowed := reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en"]`)
	out := rebuild(t, narrowed)

	_, _, err := out.Write()
	if err == nil {
		t.Fatal("loc removed a file somebody had edited")
	}
	if got := err.Error(); !strings.Contains(got, "modified since loc wrote it") {
		t.Fatalf("err = %q", got)
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Fatal("the edited file was removed anyway")
	}
}

// A relative -c and an absolute one must produce the same record bytes.
func TestTheRecordDoesNotDependOnHowTheConfigWasSpelled(t *testing.T) {
	relative := buildAt(t, "i18n/config.cue")
	absolute := buildAt(t, "")

	if !slices.Equal(relative, absolute) {
		t.Fatalf("the record differs by invocation:\n%s\n%s", relative, absolute)
	}
	var man generate.Manifest
	if err := json.Unmarshal(relative, &man); err != nil {
		t.Fatal(err)
	}
	if man.SchemaVersion != generate.ManifestSchemaVersion {
		t.Fatalf("schemaVersion = %d", man.SchemaVersion)
	}
	if man.Target != "backend" {
		t.Fatalf("target = %q", man.Target)
	}
	for rel := range man.Files {
		if filepath.IsAbs(rel) || strings.Contains(rel, "..") {
			t.Fatalf("the record names %q, which is not inside the target directory", rel)
		}
	}
}

// sumOf is the sha256 a record would carry for the file at path.
func sumOf(t *testing.T, path string) string {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// tamper rewrites the record in dir as JSON, like a hand edit or a bad merge.
func tamper(t *testing.T, dir string, edit func(man map[string]any)) {
	t.Helper()

	name := filepath.Join(dir, generate.ManifestName)
	raw, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	var man map[string]any
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatal(err)
	}
	edit(man)
	body, err := json.Marshal(man)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A tampered record never removes a file: build, check and clean all stop.
func TestABadRecordRemovesNothing(t *testing.T) {
	for _, tc := range []struct {
		name  string
		want  string
		spoil func(t *testing.T, pkg, dir string) (victim string)
	}{
		{"a path above the tree", `names "../../precious.txt", which is outside`, func(t *testing.T, pkg, dir string) string {
			victim := filepath.Join(dir, "precious.txt")
			writeFile(t, victim, []byte("mine\n"))
			tamper(t, pkg, func(man map[string]any) {
				man["files"].(map[string]any)["../../precious.txt"] = sumOf(t, victim)
			})
			return victim
		}},
		{"an absolute path", "precious.txt\", which is outside", func(t *testing.T, pkg, dir string) string {
			victim := filepath.Join(dir, "precious.txt")
			writeFile(t, victim, []byte("mine\n"))
			tamper(t, pkg, func(man map[string]any) {
				man["files"].(map[string]any)[victim] = sumOf(t, victim)
			})
			return victim
		}},
		{"an empty hash", `gives "messages.go" the hash "", which is not a sha256; remove that record by hand, and any file it lists that loc wrote`, func(t *testing.T, pkg, _ string) string {
			tamper(t, pkg, func(man map[string]any) {
				man["files"].(map[string]any)["messages.go"] = ""
			})
			return filepath.Join(pkg, "messages.go")
		}},
		{"a null hash", `gives "messages.go" the hash "", which is not a sha256; remove that record by hand, and any file it lists that loc wrote`, func(t *testing.T, pkg, _ string) string {
			tamper(t, pkg, func(man map[string]any) {
				man["files"].(map[string]any)["messages.go"] = nil
			})
			return filepath.Join(pkg, "messages.go")
		}},
		{"a hash that is not a sha256", `gives "messages.go" the hash "abc", which is not a sha256; remove that record by hand, and any file it lists that loc wrote`, func(t *testing.T, pkg, _ string) string {
			tamper(t, pkg, func(man map[string]any) {
				man["files"].(map[string]any)["messages.go"] = "abc"
			})
			return filepath.Join(pkg, "messages.go")
		}},
		{"another schema version", generate.ManifestName + ": schemaVersion 1, want 2; remove that record by hand, and any file it lists that loc wrote", func(t *testing.T, pkg, _ string) string {
			tamper(t, pkg, func(man map[string]any) {
				man["schemaVersion"] = 1
				man["files"].(map[string]any)["messages.go"] = sumOf(t, filepath.Join(pkg, "messages.go"))
			})
			return filepath.Join(pkg, "messages.go")
		}},
		{"a record that is not JSON", generate.ManifestName + ": invalid character", func(t *testing.T, pkg, _ string) string {
			writeFile(t, filepath.Join(pkg, generate.ManifestName), []byte("{not json"))
			return filepath.Join(pkg, "messages.go")
		}},
		{"a path through a symlink out of the tree", `names "evil/victim.txt", which resolves to`, func(t *testing.T, pkg, dir string) string {
			victim := filepath.Join(dir, "outside", "victim.txt")
			writeFile(t, victim, []byte("mine\n"))
			if err := os.Symlink(filepath.Dir(victim), filepath.Join(pkg, "evil")); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}
			tamper(t, pkg, func(man map[string]any) {
				man["files"].(map[string]any)["evil/victim.txt"] = sumOf(t, victim)
			})
			return victim
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, outs, dir := shared(t)
			if _, _, err := outs.Write(); err != nil {
				t.Fatal(err)
			}
			pkg := cfg.AbsTargetOutput(message.TargetBackend)
			victim := tc.spoil(t, pkg, dir)

			// Dropping pl gives both trees something a good record would remove.
			next := rebuild(t, reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en"]`))
			if _, _, err := next.Diff(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("check: err = %v, want %q", err, tc.want)
			}
			written, removed, err := next.Write()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("build: err = %v, want %q", err, tc.want)
			}
			if written != 0 || len(removed) != 0 {
				t.Fatalf("wrote %d and removed %v before refusing", written, removed)
			}
			removed, err = generate.Clean(cfg)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("clean: err = %v, want %q", err, tc.want)
			}
			if len(removed) != 0 {
				t.Fatalf("clean removed %v before refusing", removed)
			}
			for _, keep := range []string{
				victim,
				filepath.Join(pkg, "locales", "pl.json"),
				filepath.Join(dir, "i18n", "generated", "web", "pl", "common.json"),
			} {
				if _, err := os.Stat(keep); err != nil {
					t.Errorf("%s: %v", keep, err)
				}
			}
		})
	}
}

// A file at a newly produced path that no record names is not overwritten.
func TestAFileLocNeverWroteIsNotOverwritten(t *testing.T) {
	cfg, outs, _ := shared(t)
	if _, _, err := outs.Write(); err != nil {
		t.Fatal(err)
	}

	// The hand-written locales/de.json is in no record; adding de makes loc produce it.
	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	mine := filepath.Join(pkg, "locales", "de.json")
	writeFile(t, filepath.Join(cfg.Dir, "locales", "de", "common.yaml"), []byte("greeting:\n  value: \"Hallo\"\n  description: \"d\"\n"))
	writeFile(t, filepath.Join(cfg.Dir, "locales", "de", "tray.yaml"), []byte("open:\n  value: \"Offnen\"\n  description: \"d\"\n"))
	next := rebuild(t, reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en", "pl", "de"]`))

	stale, modified, err := next.Diff()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(modified, []string{"../pkg/messages/locales/de.json"}) {
		t.Fatalf("modified = %v, want the hand-written de.json", modified)
	}
	if slices.Contains(stale, "../pkg/messages/locales/de.json") {
		t.Fatalf("stale = %v names a file build refuses to touch", stale)
	}

	_, _, err = next.Write()
	if err == nil || !strings.Contains(err.Error(), "is not in loc's record") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	body, readErr := os.ReadFile(mine)
	if readErr != nil || string(body) != "{}\n" {
		t.Fatalf("de.json = %q, %v; the hand-written file was overwritten", body, readErr)
	}
}

// One tree's refusal stops every tree before anything is written or removed.
func TestARefusalInOneTreeTouchesNoOther(t *testing.T) {
	cfg, outs, dir := shared(t)
	if _, _, err := outs.Write(); err != nil {
		t.Fatal(err)
	}
	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	writeFile(t, filepath.Join(pkg, "locales", "pl.json"), []byte(`{"mine":"now"}`))

	written, removed, err := rebuild(t, reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en"]`)).Write()
	if err == nil || !strings.Contains(err.Error(), "../pkg/messages/locales/pl.json was modified since loc wrote it") {
		t.Fatalf("err = %v, want a refusal naming the edited file", err)
	}
	if written != 0 || len(removed) != 0 {
		t.Fatalf("wrote %d and removed %v before refusing", written, removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "i18n", "generated", "web", "pl", "common.json")); err != nil {
		t.Fatal("the web tree lost its pl file although the backend tree refused")
	}
}

// Pruning only removes directories a removal emptied; other empty directories stay.
func TestAnUnrelatedEmptyDirectorySurvives(t *testing.T) {
	cfg, outs, _ := shared(t)
	if _, _, err := outs.Write(); err != nil {
		t.Fatal(err)
	}
	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	keep := filepath.Join(pkg, "testdata")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}

	_, removed, err := rebuild(t, reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en"]`)).Write()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(removed, "../pkg/messages/locales/pl.json") {
		t.Fatalf("removed = %v, want pl.json", removed)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("a hand-made empty directory was pruned: %v", err)
	}
}

// Under a relative -c the emptied locale directory is still removed.
func TestARelativeConfigStillPrunes(t *testing.T) {
	_, _, dir := shared(t)
	t.Chdir(dir)

	cfg, err := config.Load(context.Background(), "i18n/config.cue")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := rebuild(t, cfg).Write(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := rebuild(t, reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en"]`)).Write(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "i18n", "generated", "web", "pl")); !os.IsNotExist(err) {
		t.Fatalf("generated/web/pl survived its last file: %v", err)
	}
}

const bothAtDefault = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]
roots: ["locales"]
output: "generated"
namespaces: {
	common: {targets: ["web"]}
	tray:   {targets: ["backend"]}
}
go: {package: "messages"}
`

// configure replaces the whole config and loads it again.
func configure(t *testing.T, cfg *config.Config, body string) *config.Config {
	t.Helper()

	path := filepath.Join(cfg.Dir, "config.cue")
	writeFile(t, path, []byte(body))
	next, err := config.Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

// The next build clears a dropped target's default tree by its record.
func TestADroppedTargetsTreeIsCleared(t *testing.T) {
	cfg, _, dir := shared(t)
	both := configure(t, cfg, bothAtDefault)
	if _, _, err := rebuild(t, both).Write(); err != nil {
		t.Fatal(err)
	}
	backend := filepath.Join(dir, "i18n", "generated", "backend")
	notes := filepath.Join(backend, "NOTES.txt")
	writeFile(t, notes, []byte("mine\n"))

	webOnly := configure(t, both, webAtDefault)
	next := rebuild(t, webOnly)
	gone := []string{
		"generated/backend/embed_gen.go",
		"generated/backend/keys_gen.go",
		"generated/backend/loc-manifest.json",
		"generated/backend/locales/en.json",
		"generated/backend/locales/pl.json",
		"generated/backend/messages_gen.go",
	}

	stale, modified, err := next.Diff()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range gone {
		if !slices.Contains(stale, want) {
			t.Errorf("stale = %v, want %s", stale, want)
		}
	}
	if len(modified) != 0 {
		t.Fatalf("modified = %v", modified)
	}

	_, removed, err := next.Write()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(removed, gone) {
		t.Fatalf("removed = %v, want %v", removed, gone)
	}
	if _, err := os.Stat(notes); err != nil {
		t.Fatal("a file loc never wrote went with the retired tree")
	}
	if _, err := os.Stat(filepath.Join(backend, "locales")); !os.IsNotExist(err) {
		t.Fatalf("the emptied locales directory survived: %v", err)
	}

	stale, modified, err = rebuild(t, webOnly).Diff()
	if err != nil || len(stale)+len(modified) != 0 {
		t.Fatalf("after the build: stale %v, modified %v, err %v", stale, modified, err)
	}
}

// A target moved by outputs has its default tree cleared and the empty directory removed.
func TestAMovedTargetsOldTreeIsCleared(t *testing.T) {
	cfg, _, dir := shared(t)
	both := configure(t, cfg, bothAtDefault)
	if _, _, err := rebuild(t, both).Write(); err != nil {
		t.Fatal(err)
	}

	moved := configure(t, both, strings.Replace(bothAtDefault, `go: {package`, "outputs: {backend: \"../pkg/messages\"}\ngo: {package", 1))
	_, removed, err := rebuild(t, moved).Write()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(removed, "generated/backend/loc-manifest.json") || !slices.Contains(removed, "generated/backend/messages_gen.go") {
		t.Fatalf("removed = %v, want the old backend tree", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "i18n", "generated", "backend")); !os.IsNotExist(err) {
		t.Fatalf("the emptied old backend directory survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "pkg", "messages", "messages_gen.go")); err != nil {
		t.Fatal("the moved target was not written to its new directory")
	}
}

// Dirs names every tree a build writes or clears, retired ones included, and no ancestor.
func TestDirsNameEveryTreeABuildTouches(t *testing.T) {
	moved := strings.Replace(bothAtDefault, `go: {package`, "outputs: {backend: \"../pkg/messages\"}\ngo: {package", 1)
	for name, body := range map[string]string{"dropped": webAtDefault, "moved": moved} {
		t.Run(name, func(t *testing.T) {
			cfg, _, _ := shared(t)
			both := configure(t, cfg, bothAtDefault)
			if _, _, err := rebuild(t, both).Write(); err != nil {
				t.Fatal(err)
			}

			next := configure(t, both, body)
			outs := rebuild(t, next)
			_, removed, err := outs.Write()
			if err != nil {
				t.Fatal(err)
			}
			if len(removed) == 0 {
				t.Fatal("no tree was retired, so this proves nothing")
			}

			var trees []string
			for _, target := range next.Targets {
				trees = append(trees, outs.For(target).Dir)
			}
			// A cleared tree drops its record, which sits at the tree's root.
			retiredTrees := 0
			for _, label := range removed {
				if path.Base(label) == generate.ManifestName {
					trees = append(trees, filepath.Join(next.Dir, filepath.FromSlash(path.Dir(label))))
					retiredTrees++
				}
			}
			if retiredTrees == 0 {
				t.Fatalf("removed = %v names no record, so no retired tree is known", removed)
			}

			var dirs []string
			for _, dir := range generate.Dirs(next) {
				dirs = append(dirs, filepath.Clean(dir))
			}
			for _, tree := range trees {
				if !slices.Contains(dirs, filepath.Clean(tree)) {
					t.Errorf("Dirs does not name %s: %v", tree, dirs)
				}
			}
		})
	}
}

// A retired tree is held to the same proof: an edited file in it stops the build.
func TestAnEditedFileInARetiredTreeStopsTheBuild(t *testing.T) {
	cfg, _, dir := shared(t)
	both := configure(t, cfg, bothAtDefault)
	if _, _, err := rebuild(t, both).Write(); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(dir, "i18n", "generated", "backend", "locales", "pl.json")
	writeFile(t, edited, []byte(`{"mine":"now"}`))

	moved := configure(t, both, strings.Replace(bothAtDefault, `go: {package`, "outputs: {backend: \"../pkg/messages\"}\ngo: {package", 1))
	_, _, err := rebuild(t, moved).Write()
	if err == nil || !strings.Contains(err.Error(), "generated/backend/locales/pl.json was modified since loc wrote it") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	removed, err := generate.Clean(moved)
	if err == nil || !strings.Contains(err.Error(), "generated/backend/locales/pl.json was modified since loc wrote it") || len(removed) != 0 {
		t.Fatalf("clean: removed %v, err %v; want a refusal", removed, err)
	}
	if _, err := os.Stat(edited); err != nil {
		t.Fatal("the edited file was removed")
	}
}

// A recorded path that is now a symlink is refused, even when it leads to the right bytes.
func TestARecordedPathThatIsNowASymlinkIsRefused(t *testing.T) {
	cfg, outs, _ := shared(t)
	if _, _, err := outs.Write(); err != nil {
		t.Fatal(err)
	}
	pkg := cfg.AbsTargetOutput(message.TargetBackend)
	link := filepath.Join(pkg, "locales", "pl.json")
	body, err := os.ReadFile(link)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(pkg, "pl-copy.json"), body)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(pkg, "pl-copy.json"), link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	_, _, err = rebuild(t, reload(t, cfg, `locales: ["en", "pl"]`, `locales: ["en"]`)).Write()
	if err == nil || !strings.Contains(err.Error(), "../pkg/messages/locales/pl.json was modified since loc wrote it") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	removed, err := generate.Clean(cfg)
	if err == nil || !strings.Contains(err.Error(), "../pkg/messages/locales/pl.json was modified since loc wrote it") || len(removed) != 0 {
		t.Fatalf("clean: removed %v, err %v; want a refusal", removed, err)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Fatal("the symlink was removed")
	}
}

const webAtDefault = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web"]
roots: ["locales"]
output: "generated"
`

// webFiles is what the fixture's web target writes, relative to its directory.
var webFiles = []string{"en/common.json", "en/tray.json", "keys.gen.ts", "loc-manifest.json", "pl/common.json", "pl/tray.json", "plurals.gen.ts"}

// webBuiltAtDefault builds the fixture's web target alone into generated/web.
func webBuiltAtDefault(t *testing.T) (*config.Config, string) {
	t.Helper()

	cfg, _, dir := shared(t)
	web := configure(t, cfg, webAtDefault)
	if _, _, err := rebuild(t, web).Write(); err != nil {
		t.Fatal(err)
	}
	return web, dir
}

// When a symlink makes a moved target's old directory the new one, nothing is removed.
func TestAMoveOntoTheSameDirectoryClearsNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		// moved: tree at public/i18n, generated/web links to it; else the link goes the other way.
		moved bool
	}{
		{"the old path links to the new", true},
		{"the new path links to the old", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			web, dir := webBuiltAtDefault(t)
			gen := filepath.Join(dir, "i18n", "generated", "web")
			public := filepath.Join(dir, "public", "i18n")
			if err := os.MkdirAll(filepath.Dir(public), 0o755); err != nil {
				t.Fatal(err)
			}
			link, dest := public, gen
			if tc.moved {
				if err := os.Rename(gen, public); err != nil {
					t.Fatal(err)
				}
				link, dest = gen, public
			}
			if err := os.Symlink(dest, link); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}

			moved := configure(t, web, webAtDefault+"outputs: {web: \"../public/i18n\"}\n")
			for build := 1; build <= 2; build++ {
				next := rebuild(t, moved)
				stale, modified, err := next.Diff()
				if err != nil || len(stale)+len(modified) != 0 {
					t.Fatalf("build %d: stale %v, modified %v, err %v", build, stale, modified, err)
				}
				written, removed, err := next.Write()
				if err != nil {
					t.Fatalf("build %d: %v", build, err)
				}
				if written != 0 || len(removed) != 0 {
					t.Fatalf("build %d: wrote %d, removed %v; want nothing", build, written, removed)
				}
			}

			info, err := os.Lstat(link)
			if err != nil || info.Mode()&os.ModeSymlink == 0 {
				t.Fatalf("the link %s did not survive: %v", link, err)
			}
			for _, rel := range webFiles {
				if _, err := os.Stat(filepath.Join(public, rel)); err != nil {
					t.Errorf("%s: %v", rel, err)
				}
			}
		})
	}
}

// Without a link the new directory is a different one, and the old tree is cleared.
func TestAMoveIntoANewDirectoryClearsTheOldTree(t *testing.T) {
	web, dir := webBuiltAtDefault(t)

	moved := configure(t, web, webAtDefault+"outputs: {web: \"../public/i18n\"}\n")
	written, removed, err := rebuild(t, moved).Write()
	if err != nil {
		t.Fatal(err)
	}
	var gone []string
	for _, rel := range webFiles {
		gone = append(gone, "generated/web/"+rel)
		if _, err := os.Stat(filepath.Join(dir, "public", "i18n", rel)); err != nil {
			t.Errorf("new tree: %v", err)
		}
	}
	if written != len(webFiles) || !slices.Equal(removed, gone) {
		t.Fatalf("wrote %d, removed %v; want %d and %v", written, removed, len(webFiles), gone)
	}
	if _, err := os.Stat(filepath.Join(dir, "i18n", "generated", "web")); !os.IsNotExist(err) {
		t.Fatalf("the old web directory survived: %v", err)
	}
}

// Only the same directory is spared: an old tree inside the new one is cleared.
func TestAnOldTreeInsideTheNewOneIsCleared(t *testing.T) {
	const nested = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["backend"]
roots: ["locales"]
output: "pkg/locales"
go: {package: "messages"}
`
	cfg, _, dir := shared(t)
	before := configure(t, cfg, nested)
	if _, _, err := rebuild(t, before).Write(); err != nil {
		t.Fatal(err)
	}

	moved := configure(t, before, nested+"outputs: {backend: \"pkg\"}\n")
	_, removed, err := rebuild(t, moved).Write()
	if err != nil {
		t.Fatal(err)
	}
	gone := []string{
		"pkg/locales/backend/embed_gen.go",
		"pkg/locales/backend/keys_gen.go",
		"pkg/locales/backend/loc-manifest.json",
		"pkg/locales/backend/locales/en.json",
		"pkg/locales/backend/locales/pl.json",
		"pkg/locales/backend/messages_gen.go",
	}
	if !slices.Equal(removed, gone) {
		t.Fatalf("removed = %v, want %v", removed, gone)
	}
	pkg := filepath.Join(dir, "i18n", "pkg")
	if _, err := os.Stat(filepath.Join(pkg, "locales", "backend")); !os.IsNotExist(err) {
		t.Fatalf("the old backend directory survived: %v", err)
	}
	for _, rel := range []string{"embed_gen.go", "keys_gen.go", "loc-manifest.json", "locales/en.json", "locales/pl.json", "messages_gen.go"} {
		if _, err := os.Stat(filepath.Join(pkg, rel)); err != nil {
			t.Errorf("new tree: %v", err)
		}
	}

	written, removed, err := rebuild(t, moved).Write()
	if err != nil || written != 0 || len(removed) != 0 {
		t.Fatalf("second build: wrote %d, removed %v, err %v; want nothing", written, removed, err)
	}
}

// An old web path linked to the backend's live directory is not cleared as an old tree.
func TestAnOldPathLinkedToAnotherTargetsTreeClearsNothing(t *testing.T) {
	const backendOnly = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["backend"]
roots: ["locales"]
output: "generated"
go: {package: "messages"}
`
	for _, tc := range []struct {
		name  string
		extra string
	}{
		{"a redirected target", "outputs: {backend: \"../pkg/messages\"}\n"},
		{"a target at its default location", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _, dir := shared(t)
			backend := configure(t, cfg, backendOnly+tc.extra)
			if _, _, err := rebuild(t, backend).Write(); err != nil {
				t.Fatal(err)
			}
			pkg := backend.AbsTargetOutput(message.TargetBackend)
			gen := filepath.Join(dir, "i18n", "generated", "web")
			if err := os.MkdirAll(filepath.Dir(gen), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(pkg, gen); err != nil {
				t.Skipf("no symlinks here: %v", err)
			}

			for build := 1; build <= 2; build++ {
				written, removed, err := rebuild(t, backend).Write()
				if err != nil || written != 0 || len(removed) != 0 {
					t.Fatalf("build %d: wrote %d, removed %v, err %v; want nothing", build, written, removed, err)
				}
			}
			for _, rel := range []string{"embed_gen.go", "keys_gen.go", "loc-manifest.json", "locales/en.json", "locales/pl.json", "messages_gen.go"} {
				if _, err := os.Stat(filepath.Join(pkg, rel)); err != nil {
					t.Errorf("the backend's own output went: %v", err)
				}
			}
		})
	}
}

// Two old paths to one directory are cleared once, through the symlink, so the link survives.
func TestTwoOldPathsThatAreOneDirectoryAreClearedOnce(t *testing.T) {
	for _, tc := range []struct {
		name string
		// link: the old path that is a symlink; gone: what build or clean removes under its name.
		link, dest string
		gone       []string
	}{
		{"the web path links to the backend's", "web", "backend", []string{
			"generated/web/embed_gen.go",
			"generated/web/keys_gen.go",
			"generated/web/loc-manifest.json",
			"generated/web/locales/en.json",
			"generated/web/locales/pl.json",
			"generated/web/messages_gen.go",
		}},
		{"the backend path links to the web's", "backend", "web", []string{
			"generated/backend/en/common.json",
			"generated/backend/keys.gen.ts",
			"generated/backend/loc-manifest.json",
			"generated/backend/pl/common.json",
			"generated/backend/plurals.gen.ts",
		}},
	} {
		// The moved trees were never built, so clean finds exactly the old tree too.
		for _, clean := range []bool{false, true} {
			name := tc.name + ", build"
			if clean {
				name = tc.name + ", clean"
			}
			t.Run(name, func(t *testing.T) {
				cfg, _, dir := shared(t)
				both := configure(t, cfg, bothAtDefault)
				if _, _, err := rebuild(t, both).Write(); err != nil {
					t.Fatal(err)
				}
				gen := filepath.Join(dir, "i18n", "generated")
				link := filepath.Join(gen, tc.link)
				if err := os.RemoveAll(link); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(tc.dest, link); err != nil {
					t.Skipf("no symlinks here: %v", err)
				}
				linkSurvives := func() {
					t.Helper()
					if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
						t.Fatalf("the link %s did not survive: %v", link, err)
					}
					if info, err := os.Stat(link); err != nil || !info.IsDir() {
						t.Fatalf("the link %s no longer leads to a directory: %v", link, err)
					}
				}

				moved := configure(t, both, strings.Replace(bothAtDefault, `go: {package`,
					"outputs: {backend: \"../pkg/messages\", web: \"../public\"}\ngo: {package", 1))
				if clean {
					removed, err := generate.Clean(moved)
					if err != nil {
						t.Fatal(err)
					}
					if !slices.Equal(removed, tc.gone) {
						t.Fatalf("removed = %v, want each once: %v", removed, tc.gone)
					}
					linkSurvives()
					return
				}

				next := rebuild(t, moved)
				stale, modified, err := next.Diff()
				if err != nil || len(modified) != 0 {
					t.Fatalf("modified %v, err %v", modified, err)
				}
				var old []string
				for _, p := range stale {
					if strings.HasPrefix(p, "generated/") {
						old = append(old, p)
					}
				}
				if !slices.Equal(old, tc.gone) {
					t.Fatalf("stale under generated/ = %v, want each once: %v", old, tc.gone)
				}

				_, removed, err := next.Write()
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(removed, tc.gone) {
					t.Fatalf("removed = %v, want each once: %v", removed, tc.gone)
				}
				linkSurvives()

				written, removed, err := rebuild(t, moved).Write()
				if err != nil || written != 0 || len(removed) != 0 {
					t.Fatalf("second build: wrote %d, removed %v, err %v; want nothing", written, removed, err)
				}
			})
		}
	}
}

// Two targets that are one directory through a symlink are refused before any write.
func TestTwoTargetsInOneDirectoryAreRefused(t *testing.T) {
	cfg, _, dir := shared(t)
	if err := os.Symlink(filepath.Join(dir, "pkg", "messages"), filepath.Join(dir, "public")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	linked := configure(t, cfg, strings.Replace(sharedCfg, `outputs: {backend: "../pkg/messages"}`,
		`outputs: {backend: "../pkg/messages", web: "../public"}`, 1))

	res, err := parse.Roots(context.Background(), linked.AbsRoots(), linked.Locales)
	if err != nil {
		t.Fatal(err)
	}
	_, err = generate.Build(context.Background(), linked, res.Set)
	want := `generate: targets "web" and "backend" both write into ../pkg/messages: ../public is the same directory`
	if err == nil || err.Error() != want {
		t.Fatalf("err = %v, want %s", err, want)
	}
	if _, err := generate.Clean(linked); err == nil || err.Error() != want {
		t.Fatalf("clean: err = %v, want %s", err, want)
	}
}
