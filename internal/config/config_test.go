// internal/config/config_test.go

package config_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
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

func TestOutputsRedirectsOneTarget(t *testing.T) {
	cfg := load(t, valid+"\noutputs: {backend: \"../backend/common/messages\"}\n")

	if !cfg.Redirected(message.TargetBackend) {
		t.Fatal("backend is not reported as redirected")
	}
	if cfg.Redirected(message.TargetWeb) {
		t.Fatal("web is reported as redirected without an entry")
	}

	want := filepath.Join(filepath.Dir(cfg.Dir), "backend", "common", "messages")
	if got := cfg.AbsTargetOutput(message.TargetBackend); got != want {
		t.Fatalf("backend dir = %q, want %q", got, want)
	}
	if got, want := cfg.AbsTargetOutput(message.TargetWeb), filepath.Join(cfg.Dir, "generated", "web"); got != want {
		t.Fatalf("web dir = %q, want %q", got, want)
	}
	if got := cfg.TargetLabel(message.TargetBackend); got != "../backend/common/messages" {
		t.Fatalf("backend label = %q", got)
	}
}

func TestAnUnknownTargetKeyInOutputsIsRejected(t *testing.T) {
	if got := loadErr(t, valid+"\noutputs: {bakend: \"../x\"}\n"); !strings.Contains(got, "outputs.bakend") {
		t.Fatalf("err = %q, want it to name outputs.bakend", got)
	}
}

// `output: "."` used to be caught by clean; it has to be caught at load, or
// the sweep is what finds the typo.
func TestOutputMayNotBeTheConfigDirectory(t *testing.T) {
	if got := loadErr(t, valid+"\noutput: \".\"\n"); !strings.Contains(got, "not below the config directory") {
		t.Fatalf("err = %q", got)
	}
}

func TestOutputMayNotBeAboveTheConfigDirectory(t *testing.T) {
	if got := loadErr(t, valid+"\noutput: \"..\"\n"); !strings.Contains(got, "not below the config directory") {
		t.Fatalf("err = %q", got)
	}
}

func TestOutputMayNotBeAnAbsolutePathElsewhere(t *testing.T) {
	body := valid + "\noutput: " + strconv.Quote(t.TempDir()) + "\n"
	if got := loadErr(t, body); !strings.Contains(got, "not below the config directory") {
		t.Fatalf("err = %q", got)
	}
}

// A target directory holding a canonical root would make the removal sweep
// point at the source it was built from.
func TestATargetMayNotHoldACanonicalRoot(t *testing.T) {
	body := `
defaultLocale: "en"
locales: ["en"]
targets: ["web"]
roots: ["locales"]
outputs: {web: "."}
`
	if got := loadErr(t, body); !strings.Contains(got, "config directory") {
		t.Fatalf("err = %q", got)
	}
}

func TestTwoTargetsMayNotShareADirectory(t *testing.T) {
	body := valid + "\noutputs: {web: \"../out\", backend: \"../out/inner\"}\n"
	if got := loadErr(t, body); !strings.Contains(got, "both write into") {
		t.Fatalf("err = %q", got)
	}
}

// A redirect is acted on even for a target that is not built, and nothing
// else checks where it points.
func TestOutputsMayNotNameATargetThatIsNotBuilt(t *testing.T) {
	body := `
defaultLocale: "en"
locales: ["en"]
targets: ["web"]
roots: ["locales"]
outputs: {backend: "../x"}
`
	if got := loadErr(t, body); !strings.Contains(got, `outputs names "backend", which is not in targets`) {
		t.Fatalf("err = %q", got)
	}
}

// The default -c is relative, and an absolute entry cannot be related to a
// relative config directory without resolving both.
func TestARedirectToTheConfigDirectoryIsCaughtHoweverItIsSpelled(t *testing.T) {
	for _, tc := range []struct{ name, suffix string }{
		{"absolute", ""},
		{"trailing slash", "/"},
		{"through a parent", "/x/.."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			body := `
defaultLocale: "en"
locales: ["en"]
targets: ["backend"]
roots: ["../shared/locales"]
outputs: {backend: ` + strconv.Quote(dir+tc.suffix) + `}
`
			if err := os.WriteFile(filepath.Join(dir, "config.cue"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Chdir(dir)

			_, err := config.Load("config.cue")
			if err == nil || !strings.Contains(err.Error(), "writes into the config directory") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

// Under a relative -c, an absolute output inside the config directory is as
// good as the relative spelling of the same directory.
func TestAnAbsoluteOutputInsideTheConfigDirectoryLoads(t *testing.T) {
	dir := t.TempDir()
	body := valid + "\noutput: " + strconv.Quote(filepath.Join(dir, "generated")) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.cue"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if _, err := config.Load("config.cue"); err != nil {
		t.Fatal(err)
	}
}

// Nothing prunes the output tree's own files by name, so a root under it is
// removed on the next build.
func TestOutputMayNotHoldASourceRoot(t *testing.T) {
	if got := loadErr(t, valid+"\noutput: \"i18n\"\n"); !strings.Contains(got, "holds the source root") {
		t.Fatalf("err = %q", got)
	}
}

func TestATargetMayNotHoldASourceRoot(t *testing.T) {
	if got := loadErr(t, valid+"\noutputs: {backend: \"i18n\"}\n"); !strings.Contains(got, "holds the source root") {
		t.Fatalf("err = %q", got)
	}
}

func TestATargetMayNotHoldAScanRoot(t *testing.T) {
	body := valid + "\nscan: [\"pkg/src\"]\noutputs: {backend: \"pkg\"}\n"
	if got := loadErr(t, body); !strings.Contains(got, "holds the source root") {
		t.Fatalf("err = %q", got)
	}
}

func TestTwoTargetsMayNotShareADirectoryInEitherOrder(t *testing.T) {
	body := valid + "\noutputs: {web: \"../out/inner\", backend: \"../out\"}\n"
	if got := loadErr(t, body); !strings.Contains(got, "both write into") {
		t.Fatalf("err = %q", got)
	}
}

// The output tree's sweep would remove a redirected target's files on every
// build, and check would then call them stale.
func TestARedirectedTargetMayNotSitInsideTheOutputTree(t *testing.T) {
	if got := loadErr(t, valid+"\noutputs: {backend: \"generated/pkg\"}\n"); !strings.Contains(got, "inside the output tree") {
		t.Fatalf("err = %q", got)
	}
}
