// internal/shared/suggest_test.go

package shared_test

import (
	"slices"
	"testing"

	"github.com/smegg99/s99locale/internal/shared"
)

func TestNoSuggestionOutsideTheVocabulary(t *testing.T) {
	got := detect(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Reticulating splines\"\n",
		"pl/a.yaml": "x:\n  value: \"Reticulating splines\"\n",
		"en/b.yaml": "y:\n  value: \"Reticulating splines\"\n",
		"pl/b.yaml": "y:\n  value: \"Reticulating splines\"\n",
	})

	c := find(t, got, "a.x", "b.y")
	if c.Confidence != shared.High {
		t.Fatalf("confidence = %s", c.Confidence)
	}
	// The group is real; the name is not derivable, so none is offered.
	if c.Suggested != "" {
		t.Fatalf("invented a name: %q", c.Suggested)
	}
}

// An interpolated message carries context a generic name would lose.
func TestNoSuggestionForAMessageWithPlaceholders(t *testing.T) {
	got := detect(t, map[string]string{
		"en/a.yaml": "x:\n  value: \"Close {what}\"\n  placeholders:\n    what: {type: string}\n",
		"pl/a.yaml": "x:\n  value: \"Close {what}\"\n  placeholders:\n    what: {type: string}\n",
		"en/b.yaml": "y:\n  value: \"Close {what}\"\n  placeholders:\n    what: {type: string}\n",
		"pl/b.yaml": "y:\n  value: \"Close {what}\"\n  placeholders:\n    what: {type: string}\n",
	})

	if c := find(t, got, "a.x", "b.y"); c.Suggested != "" {
		t.Fatalf("suggested %q for an interpolated message", c.Suggested)
	}
}

func TestNoSuggestionWhenTheTargetKeyExists(t *testing.T) {
	got := detect(t, map[string]string{
		"en/menu.yaml":   "accept:\n  value: \"OK\"\n",
		"pl/menu.yaml":   "accept:\n  value: \"OK\"\n",
		"en/dialog.yaml": "accept:\n  value: \"OK\"\n",
		"pl/dialog.yaml": "accept:\n  value: \"OK\"\n",
		// common.action.accept already exists with its own contract.
		"en/common.yaml": "action:\n  accept:\n    value: \"Accept\"\n",
		"pl/common.yaml": "action:\n  accept:\n    value: \"Zatwierdź\"\n",
	})

	if c := find(t, got, "dialog.accept", "menu.accept"); c.Suggested != "" {
		t.Fatalf("suggested %q, which already exists with different text", c.Suggested)
	}
}

func TestSingleKeyIsNotACandidate(t *testing.T) {
	got := detect(t, map[string]string{
		"en/a.yaml": "only:\n  value: \"OK\"\n",
		"pl/a.yaml": "only:\n  value: \"OK\"\n",
	})

	if len(got) != 0 {
		t.Fatalf("candidates = %v, want none", summarise(got))
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	files := map[string]string{
		"en/menu.yaml":   "accept:\n  value: \"OK\"\n",
		"pl/menu.yaml":   "accept:\n  value: \"OK\"\n",
		"en/dialog.yaml": "accept:\n  value: \"OK\"\n",
		"pl/dialog.yaml": "accept:\n  value: \"OK\"\n",
		"en/z.yaml":      "cancel:\n  value: \"Cancel\"\n",
		"pl/z.yaml":      "cancel:\n  value: \"Anuluj\"\n",
		"en/y.yaml":      "cancel:\n  value: \"Cancel\"\n",
		"pl/y.yaml":      "cancel:\n  value: \"Anuluj\"\n",
	}

	first, second := summarise(detect(t, files)), summarise(detect(t, files))
	if !slices.Equal(first, second) {
		t.Fatalf("two runs disagreed:\n%v\n%v", first, second)
	}
	// High before low, so the useful findings are not buried.
	if len(first) < 2 {
		t.Fatalf("expected two groups, got %v", first)
	}
}

// A key already in the shared namespace is the suggestion.
func TestSuggestsTheExistingSharedKey(t *testing.T) {
	got := detect(t, map[string]string{
		"en/common.yaml": "close:\n  value: \"Close\"\n",
		"pl/common.yaml": "close:\n  value: \"Zamknij\"\n",
		"en/demo.yaml":   "dismiss:\n  value: \"Close\"\n",
		"pl/demo.yaml":   "dismiss:\n  value: \"Zamknij\"\n",
	})

	c := find(t, got, "common.close", "demo.dismiss")
	if c.Suggested != "common.close" {
		t.Fatalf("suggested %q, want the key that is already shared", c.Suggested)
	}
}

// With two keys already shared, the controlled vocabulary picks the suggestion.
func TestVocabularyBreaksTheTieWhenBothAreShared(t *testing.T) {
	got := detect(t, map[string]string{
		"en/common.yaml": "close:\n  value: \"Close\"\ndismiss:\n  value: \"Close\"\n",
		"pl/common.yaml": "close:\n  value: \"Zamknij\"\ndismiss:\n  value: \"Zamknij\"\n",
	})

	c := find(t, got, "common.close", "common.dismiss")
	if c.Suggested != "common.action.close" {
		t.Fatalf("suggested %q, want the canonical vocabulary name", c.Suggested)
	}
}
