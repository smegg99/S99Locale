// internal/config/config_test.go

package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
)

const valid = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]
roots: ["../S99AB/i18n/locales", "i18n/locales"]
namespaces: {
	common: {targets: ["web"]}
	emails: {targets: ["backend"]}
}
`

func write(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.cue")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func load(t *testing.T, body string) *config.Config {
	t.Helper()
	cfg, err := config.Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func loadErr(t *testing.T, body string) string {
	t.Helper()
	_, err := config.Load(write(t, body))
	if err == nil {
		t.Fatal("expected an error")
	}
	return err.Error()
}

func TestDefaultsApply(t *testing.T) {
	cfg := load(t, valid)

	if cfg.Output != "generated" {
		t.Fatalf("output = %q", cfg.Output)
	}
	if cfg.Go.Package != "messages" {
		t.Fatalf("go package = %q", cfg.Go.Package)
	}
}

func TestUnknownTargetIsRejectedByTheSchema(t *testing.T) {
	body := strings.Replace(valid, `targets: ["web", "backend"]`, `targets: ["web", "mobile"]`, 1)
	if got := loadErr(t, body); !strings.Contains(got, "mobile") {
		t.Fatalf("error does not name the bad target: %s", got)
	}
}

func TestDefaultLocaleMustBeListed(t *testing.T) {
	body := strings.Replace(valid, `defaultLocale: "en"`, `defaultLocale: "de"`, 1)
	if got := loadErr(t, body); !strings.Contains(got, "defaultLocale") {
		t.Fatalf("error = %s", got)
	}
}

// A locale with no plural rule would silently produce wrong counted strings.
func TestLocaleWithoutAPluralRuleIsRejected(t *testing.T) {
	body := strings.Replace(valid, `locales: ["en", "pl"]`, `locales: ["en", "xx"]`, 1)
	if got := loadErr(t, body); !strings.Contains(got, "plural rule") {
		t.Fatalf("error = %s", got)
	}
}

func TestDuplicateLocaleIsRejected(t *testing.T) {
	body := strings.Replace(valid, `locales: ["en", "pl"]`, `locales: ["en", "pl", "en"]`, 1)
	if got := loadErr(t, body); !strings.Contains(got, "twice") {
		t.Fatalf("error = %s", got)
	}
}

func TestNamespaceCannotTargetSomethingNotBuilt(t *testing.T) {
	body := strings.Replace(valid, `targets: ["web", "backend"]`, `targets: ["web"]`, 1)
	if got := loadErr(t, body); !strings.Contains(got, "backend") {
		t.Fatalf("error = %s", got)
	}
}

func TestEmptyRootsIsRejected(t *testing.T) {
	body := strings.Replace(valid, `roots: ["../S99AB/i18n/locales", "i18n/locales"]`, `roots: []`, 1)
	if loadErr(t, body) == "" {
		t.Fatal("empty roots was accepted")
	}
}

// An unlisted namespace reaches every target.
func TestUnlistedNamespaceReachesEverything(t *testing.T) {
	cfg := load(t, valid)

	got := cfg.TargetsFor("somethingNew")
	if !slices.Equal(got, cfg.Targets) {
		t.Fatalf("targets = %v, want all of %v", got, cfg.Targets)
	}
}

func TestNamespaceFiltering(t *testing.T) {
	cfg := load(t, valid)

	if got := cfg.TargetsFor("common"); !slices.Equal(got, []message.Target{message.TargetWeb}) {
		t.Fatalf("common = %v", got)
	}
	if got := cfg.TargetsFor("emails"); !slices.Equal(got, []message.Target{message.TargetBackend}) {
		t.Fatalf("emails = %v", got)
	}
}

// loc must run from anywhere, so relative paths resolve against the config.
func TestRelativePathsResolveAgainstTheConfig(t *testing.T) {
	path := write(t, valid)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Dir(path)
	if got := cfg.AbsOutput(); got != filepath.Join(dir, "generated") {
		t.Fatalf("output = %q", got)
	}
	roots := cfg.AbsRoots()
	if len(roots) != 2 || !filepath.IsAbs(roots[0]) {
		t.Fatalf("roots = %v", roots)
	}
}

// The default is relative to the config file, which already lives in i18n/.
func TestDefaultOutputLandsBesideTheConfig(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "i18n")

	if err := os.MkdirAll(filepath.Join(nested, "locales", "en"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(nested, "config.cue")
	body := "defaultLocale: \"en\"\nlocales: [\"en\"]\ntargets: [\"web\"]\nroots: [\"locales\"]\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.AbsOutput(), filepath.Join(nested, "generated"); got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}
