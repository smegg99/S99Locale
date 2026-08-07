// internal/shared/shared_test.go

package shared_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/shared"
)

const cfgBody = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web"]
roots: ["locales"]
`

func detect(t *testing.T, files map[string]string) []shared.Candidate {
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
	if err := os.WriteFile(path, []byte(cfgBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := config.Load(context.Background(), path)
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
	return shared.Detect(cfg, res.Set)
}

func find(t *testing.T, got []shared.Candidate, keys ...string) shared.Candidate {
	t.Helper()
	slices.Sort(keys)

	for _, c := range got {
		if slices.Equal(c.Keys, keys) {
			return c
		}
	}
	t.Fatalf("no candidate for %v; got %v", keys, summarise(got))
	return shared.Candidate{}
}

func absent(t *testing.T, got []shared.Candidate, keys ...string) {
	t.Helper()
	slices.Sort(keys)

	for _, c := range got {
		if slices.Equal(c.Keys, keys) {
			t.Fatalf("%v was reported as %s: %s", keys, c.Confidence, c.Reason)
		}
	}
}

func summarise(got []shared.Candidate) []string {
	var out []string
	for _, c := range got {
		out = append(out, string(c.Confidence)+":"+slicesJoin(c.Keys))
	}
	return out
}

func slicesJoin(in []string) string {
	out := ""
	for i, s := range in {
		if i > 0 {
			out += "+"
		}
		out += s
	}
	return out
}

// Only the whole contract matching in every locale earns high confidence.
func TestExactDuplicateAcrossEveryLocale(t *testing.T) {
	got := detect(t, map[string]string{
		"en/menu.yaml":   "accept:\n  value: \"OK\"\n",
		"pl/menu.yaml":   "accept:\n  value: \"OK\"\n",
		"en/dialog.yaml": "accept:\n  value: \"OK\"\n",
		"pl/dialog.yaml": "accept:\n  value: \"OK\"\n",
	})

	c := find(t, got, "dialog.accept", "menu.accept")
	if c.Confidence != shared.High {
		t.Fatalf("confidence = %s, want high: %s", c.Confidence, c.Reason)
	}
	if c.Suggested != "common.action.accept" {
		t.Fatalf("suggested = %q", c.Suggested)
	}
	if c.Translations["pl"] != "OK" {
		t.Fatalf("translations = %v", c.Translations)
	}
}

// Both are "Open" in English but differ in Polish, so the finding is low.
func TestSameEnglishDifferentPolishIsNotHighConfidence(t *testing.T) {
	got := detect(t, map[string]string{
		"en/file.yaml":  "open:\n  value: \"Open\"\n",
		"pl/file.yaml":  "open:\n  value: \"Otwórz\"\n",
		"en/store.yaml": "open:\n  value: \"Open\"\n",
		"pl/store.yaml": "open:\n  value: \"Otwarte\"\n",
	})

	c := find(t, got, "file.open", "store.open")
	if c.Confidence != shared.Low {
		t.Fatalf("confidence = %s, want low", c.Confidence)
	}
	// No suggestion: a low finding must not propose a merge.
	if c.Suggested != "" {
		t.Fatalf("suggested %q for a low-confidence group", c.Suggested)
	}
}

func TestDifferentPlaceholdersAreNotDuplicates(t *testing.T) {
	got := detect(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Hello {name}\"\n  placeholders:\n    name: {type: string}\n",
		"pl/a.yaml": "x:\n  value: \"Hello {name}\"\n  placeholders:\n    name: {type: string}\n",
		"en/b.yaml": "y:\n  value: \"Hello {name}\"\n  placeholders:\n    name: {type: integer}\n",
		"pl/b.yaml": "y:\n  value: \"Hello {name}\"\n  placeholders:\n    name: {type: integer}\n",
	})

	// The text matches everywhere but the contract differs, so it is not a safe merge.
	c := find(t, got, "a.x", "b.y")
	if c.Confidence == shared.High {
		t.Fatal("a placeholder type difference was called a safe duplicate")
	}
	if c.Suggested != "" {
		t.Fatalf("suggested %q for a group with differing contracts", c.Suggested)
	}
	if !strings.Contains(c.Reason, "contract") {
		t.Fatalf("reason does not say why: %q", c.Reason)
	}
}

func TestDifferentPluralStructureIsNotADuplicate(t *testing.T) {
	got := detect(t, map[string]string{
		"en/a.yaml": `
x:
  plural:
    argument: count
    forms: {one: "{count} file", other: "{count} files"}
  placeholders:
    count: {type: integer}
`,
		"pl/a.yaml": `
x:
  plural:
    argument: count
    forms: {one: "a", few: "b", many: "c", other: "d"}
  placeholders:
    count: {type: integer}
`,
		"en/b.yaml": "y:\n  value: \"{count} file\"\n  placeholders:\n    count: {type: integer}\n",
		"pl/b.yaml": "y:\n  value: \"a\"\n  placeholders:\n    count: {type: integer}\n",
	})

	// A plural and a plain message are never the same contract, and their
	// rendered text differs too, so nothing groups them.
	absent(t, got, "a.x", "b.y")
}

// Identical text with different descriptions is downgraded.
func TestDifferentDescriptionsDowngradeToMedium(t *testing.T) {
	got := detect(t, map[string]string{
		"en/menu.yaml":   "accept:\n  value: \"OK\"\n  description: \"Menu confirm\"\n",
		"pl/menu.yaml":   "accept:\n  value: \"OK\"\n  description: \"Menu confirm\"\n",
		"en/dialog.yaml": "accept:\n  value: \"OK\"\n  description: \"Dialog confirm\"\n",
		"pl/dialog.yaml": "accept:\n  value: \"OK\"\n  description: \"Dialog confirm\"\n",
	})

	c := find(t, got, "dialog.accept", "menu.accept")
	if c.Confidence != shared.Medium {
		t.Fatalf("confidence = %s, want medium", c.Confidence)
	}
	if c.Reason == "" {
		t.Fatal("a downgrade with no reason is not actionable")
	}
}
