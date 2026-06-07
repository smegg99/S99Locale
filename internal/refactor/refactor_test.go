// internal/refactor/refactor_test.go

package refactor_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/refactor"
	"github.com/smegg99/s99locale/internal/scan"
)

const cfgBody = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web"]
roots: ["locales"]
scan: ["src"]
dynamicKeys: ["status."]
`

// project writes locale files plus source, and returns everything a plan needs.
func project(t *testing.T, locales, src map[string]string) (*config.Config, *parse.Result, *scan.Result) {
	t.Helper()
	dir := t.TempDir()

	write := func(sub string, files map[string]string) {
		for name, body := range files {
			full := filepath.Join(dir, sub, name)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	write("locales", locales)
	write("src", src)

	path := filepath.Join(dir, "config.cue")
	if err := os.WriteFile(path, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(path)
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

	refs, err := scan.Run(scan.Options{Roots: cfg.AbsScan()})
	if err != nil {
		t.Fatal(err)
	}
	return cfg, res, refs
}

// identical is two keys that genuinely say the same thing.
var identical = map[string]string{
	"en/menu.yaml":   "accept:\n  value: \"OK\"\n",
	"pl/menu.yaml":   "accept:\n  value: \"OK\"\n",
	"en/dialog.yaml": "accept:\n  value: \"OK\"\n",
	"pl/dialog.yaml": "accept:\n  value: \"OK\"\n",
}

func TestPlanListsEveryReference(t *testing.T) {
	cfg, res, refs := project(t, identical, map[string]string{
		"a.vue": "t('menu.accept')\n",
		"b.ts":  "const x = t('dialog.accept')\nconst y = t('menu.accept')\n",
	})

	plan, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"menu.accept", "dialog.accept"}, "common.action.accept")
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Edits) != 3 {
		t.Fatalf("%d edits, want 3", len(plan.Edits))
	}

	patch := plan.Patch()
	for _, want := range []string{"a.vue", "b.ts", "-menu.accept", "+common.action.accept"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("patch missing %q:\n%s", want, patch)
		}
	}
}

// Edits sorted by file then line, so two runs produce the same patch.
func TestPatchIsDeterministic(t *testing.T) {
	cfg, res, refs := project(t, identical, map[string]string{
		"z.ts": "t('menu.accept')\n",
		"a.ts": "t('menu.accept')\nt('dialog.accept')\n",
	})

	first, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"menu.accept", "dialog.accept"}, "common.action.accept")
	if err != nil {
		t.Fatal(err)
	}
	second, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"dialog.accept", "menu.accept"}, "common.action.accept")
	if err != nil {
		t.Fatal(err)
	}

	if first.Patch() != second.Patch() {
		t.Fatalf("argument order changed the patch:\n%s\n---\n%s", first.Patch(), second.Patch())
	}
}

// The refusal that matters most: merging two messages that are not the same
// message silently changes one of them.
func TestRefusesWhenContractsDiffer(t *testing.T) {
	cfg, res, refs := project(t, map[string]string{
		"en/file.yaml":  "open:\n  value: \"Open\"\n",
		"pl/file.yaml":  "open:\n  value: \"Otwórz\"\n",
		"en/store.yaml": "open:\n  value: \"Open\"\n",
		"pl/store.yaml": "open:\n  value: \"Otwarte\"\n",
	}, map[string]string{"a.ts": "t('file.open')\n"})

	_, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"file.open", "store.open"}, "common.action.open")

	var refusal *refactor.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), "not the same message") {
		t.Fatalf("refusal = %v", err)
	}
}

func TestRefusesWhenDestinationSaysSomethingElse(t *testing.T) {
	locales := map[string]string{}
	for k, v := range identical {
		locales[k] = v
	}
	locales["en/common.yaml"] = "action:\n  accept:\n    value: \"Accept\"\n"
	locales["pl/common.yaml"] = "action:\n  accept:\n    value: \"Zatwierdź\"\n"

	cfg, res, refs := project(t, locales, map[string]string{"a.ts": "t('menu.accept')\n"})

	_, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"menu.accept", "dialog.accept"}, "common.action.accept")

	var refusal *refactor.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("refusal = %v", err)
	}
}

// Merging into a destination that already says exactly the same thing is fine:
// there is nothing to lose.
func TestAllowsMergingIntoAnIdenticalDestination(t *testing.T) {
	locales := map[string]string{}
	for k, v := range identical {
		locales[k] = v
	}
	locales["en/common.yaml"] = "action:\n  accept:\n    value: \"OK\"\n"
	locales["pl/common.yaml"] = "action:\n  accept:\n    value: \"OK\"\n"

	cfg, res, refs := project(t, locales, map[string]string{"a.ts": "t('menu.accept')\n"})

	if _, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"menu.accept", "dialog.accept"}, "common.action.accept"); err != nil {
		t.Fatalf("refused an identical destination: %v", err)
	}
}

// A dynamic key's references cannot all be found, so a patch would be a lie.
func TestRefusesDynamicKeys(t *testing.T) {
	cfg, res, refs := project(t, map[string]string{
		"en/status.yaml": "pending:\n  value: \"Pending\"\n",
		"pl/status.yaml": "pending:\n  value: \"Pending\"\n",
		"en/other.yaml":  "pending:\n  value: \"Pending\"\n",
		"pl/other.yaml":  "pending:\n  value: \"Pending\"\n",
	}, map[string]string{"a.ts": "t('other.pending')\n"})

	_, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"status.pending", "other.pending"}, "common.state.pending")

	var refusal *refactor.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), "dynamic") {
		t.Fatalf("refusal = %v", err)
	}
}

func TestRefusesAKeyMissingFromALocale(t *testing.T) {
	cfg, res, refs := project(t, map[string]string{
		"en/menu.yaml":   "accept:\n  value: \"OK\"\n",
		"en/dialog.yaml": "accept:\n  value: \"OK\"\n",
		"pl/dialog.yaml": "accept:\n  value: \"OK\"\n",
	}, map[string]string{"a.ts": "t('menu.accept')\n"})

	_, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"menu.accept", "dialog.accept"}, "common.action.accept")

	var refusal *refactor.Refusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a refusal", err)
	}
}

// The command never writes, so the manual steps have to be spelled out.
func TestNotesSayWhatToDoByHand(t *testing.T) {
	cfg, res, refs := project(t, identical, map[string]string{"a.ts": "t('menu.accept')\n"})

	plan, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"menu.accept", "dialog.accept"}, "common.action.accept")
	if err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(plan.Notes, "\n")
	if !strings.Contains(joined, "remove menu.accept") {
		t.Fatalf("notes do not say to remove the old keys:\n%s", joined)
	}
	if !strings.Contains(joined, "add common.action.accept") {
		t.Fatalf("notes do not say to add the new key:\n%s", joined)
	}
}

// No references found is worth saying: it usually means the scan roots are
// wrong, not that the keys are unused.
func TestNoReferencesIsCalledOut(t *testing.T) {
	cfg, res, refs := project(t, identical, map[string]string{})

	plan, err := refactor.PlanRename(cfg, res.Set, refs,
		[]string{"menu.accept", "dialog.accept"}, "common.action.accept")
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Edits) != 0 {
		t.Fatalf("edits = %v", plan.Edits)
	}
	if !strings.Contains(strings.Join(plan.Notes, "\n"), "no source references") {
		t.Fatalf("notes = %v", plan.Notes)
	}
}

func TestRefusesWithoutADestination(t *testing.T) {
	cfg, res, refs := project(t, identical, map[string]string{})

	if _, err := refactor.PlanRename(cfg, res.Set, refs, []string{"menu.accept"}, ""); err == nil {
		t.Fatal("accepted an empty destination")
	}
}
