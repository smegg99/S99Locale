// internal/plural/plural_test.go

package plural_test

import (
	"slices"
	"testing"

	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/plural"
)

func rule(t *testing.T, locale string) plural.Rule {
	t.Helper()
	r, err := plural.For(locale)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Polish needs four bands; English two.
func TestPolishHasFourCategories(t *testing.T) {
	want := []message.Category{message.CatOne, message.CatFew, message.CatMany, message.CatOther}
	if got := rule(t, "pl").Required; !slices.Equal(got, want) {
		t.Fatalf("pl = %v, want %v", got, want)
	}
	if got := rule(t, "en").Required; len(got) != 2 {
		t.Fatalf("en = %v, want one and other", got)
	}
}

func TestRegionIsStripped(t *testing.T) {
	if !slices.Equal(rule(t, "pl-PL").Required, rule(t, "pl").Required) {
		t.Fatal("pl-PL and pl disagree about plural bands")
	}
	if !plural.Known("en_US") {
		t.Fatal("en_US was not recognised")
	}
}

func TestUnknownLocaleIsRejected(t *testing.T) {
	if _, err := plural.For("xx"); err == nil {
		t.Fatal("an unknown locale returned a rule")
	}
	if plural.Known("xx") {
		t.Fatal("Known said yes to a locale with no rule")
	}
}

func TestMissingCategories(t *testing.T) {
	p := &message.Plural{Argument: "count", Forms: map[message.Category]string{
		message.CatOne:   "{count} głos",
		message.CatOther: "{count} głosów",
	}}

	missing := rule(t, "pl").Missing(p)
	want := []message.Category{message.CatFew, message.CatMany}
	if !slices.Equal(missing, want) {
		t.Fatalf("missing = %v, want %v", missing, want)
	}
	if got := rule(t, "en").Missing(p); len(got) != 0 {
		t.Fatalf("en reported %v missing from a one/other message", got)
	}
}

// A "few" in English is never selected, so it is an error.
func TestExtraCategoryIsReported(t *testing.T) {
	p := &message.Plural{Argument: "count", Forms: map[message.Category]string{
		message.CatOne:   "one",
		message.CatFew:   "few",
		message.CatOther: "other",
	}}

	extra := rule(t, "en").Extra(p)
	if !slices.Equal(extra, []message.Category{message.CatFew}) {
		t.Fatalf("extra = %v, want [few]", extra)
	}
	if got := rule(t, "pl").Extra(p); len(got) != 0 {
		t.Fatalf("pl called %v extra, but few is one of its own bands", got)
	}
}

// The generated pipe string and index function both rely on band order.
func TestBandsAreOrderedNotDeclarationOrder(t *testing.T) {
	p := &message.Plural{Argument: "count", Forms: map[message.Category]string{
		message.CatOther: "d",
		message.CatOne:   "b",
		message.CatMany:  "e",
		message.CatFew:   "c",
	}}

	want := []message.Category{
		message.CatOne, message.CatFew, message.CatMany, message.CatOther,
	}
	if got := rule(t, "pl").Bands(p); !slices.Equal(got, want) {
		t.Fatalf("bands = %v, want %v", got, want)
	}
}

func TestBandsSkipUndeclared(t *testing.T) {
	p := &message.Plural{Argument: "count", Forms: map[message.Category]string{
		message.CatOne:   "one",
		message.CatOther: "other",
	}}

	want := []message.Category{message.CatOne, message.CatOther}
	if got := rule(t, "en").Bands(p); !slices.Equal(got, want) {
		t.Fatalf("bands = %v, want %v", got, want)
	}
}

// A dual is a distinct band, not a special case of one or few.
func TestDualLanguagesCarryTwo(t *testing.T) {
	for _, locale := range []string{"sl", "ga", "he"} {
		rule, err := plural.For(locale)
		if err != nil {
			t.Fatalf("%s: %v", locale, err)
		}
		if !rule.Requires(message.CatTwo) {
			t.Errorf("%s does not require two: %v", locale, rule.Required)
		}
	}
}
