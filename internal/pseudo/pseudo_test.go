// internal/pseudo/pseudo_test.go

package pseudo_test

import (
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/pseudo"
)

// A mangled placeholder stops interpolating, and the screen then shows a bug
// this tool invented rather than one it found.
func TestPlaceholdersSurviveExactly(t *testing.T) {
	got := pseudo.Text("Hello {name}, you have {count} messages")

	for _, want := range []string{"{name}", "{count}"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%s was mangled: %s", want, got)
		}
	}
	// And nothing accented leaked inside the braces.
	if strings.Contains(got, "{ñáɱé}") || strings.Contains(got, "{çöüñŧ}") {
		t.Fatalf("placeholder contents were accented: %s", got)
	}
}

func TestBracketsMakeTruncationVisible(t *testing.T) {
	got := pseudo.Text("Save")

	if !strings.HasPrefix(got, "[") || !strings.HasSuffix(got, "]") {
		t.Fatalf("not bracketed: %s", got)
	}
}

// A layout that only fits English is a layout that will break on German or
// Polish, so the pseudolocale has to be longer.
func TestOutputIsLonger(t *testing.T) {
	for _, in := range []string{"Save", "Sign in", "A rather longer label than usual"} {
		got := pseudo.Text(in)
		if len([]rune(got)) <= len([]rune(in)) {
			t.Errorf("%q -> %q is not longer", in, got)
		}
	}
}

func TestAsciiIsAccentedSoUntranslatedTextStandsOut(t *testing.T) {
	got := pseudo.Text("Sign in")

	if strings.Contains(got, "Sign in") {
		t.Fatalf("text passed through unchanged: %s", got)
	}
	if !strings.ContainsAny(got, "Šïğñ") {
		t.Fatalf("no accents applied: %s", got)
	}
}

// Determinism is what keeps the byte-identical build guarantee.
func TestDeterministic(t *testing.T) {
	for range 5 {
		if pseudo.Text("Sign in") != pseudo.Text("Sign in") {
			t.Fatal("two calls disagreed")
		}
	}
}

func TestEmptyStaysEmpty(t *testing.T) {
	if got := pseudo.Text(""); got != "" {
		t.Fatalf("empty became %q", got)
	}
}

// An unmatched brace is not a placeholder, so its text is accented like any
// other. What must not happen is the rest of the string disappearing.
func TestUnmatchedBraceDoesNotSwallowTheRest(t *testing.T) {
	got := pseudo.Text("50% of {things")

	if !strings.Contains(got, "{") {
		t.Fatalf("the brace was lost: %s", got)
	}
	if !strings.Contains(got, "50%") {
		t.Fatalf("text before the brace was lost: %s", got)
	}
	// "things" accented, so the tail survived rather than being truncated.
	if !strings.Contains(got, "ŧĥïñğš") {
		t.Fatalf("text after the brace was lost: %s", got)
	}
}

// The pseudolocale's bands come from its own rule. Generating four for a
// two-band locale would produce a message no index function can address.
func TestPluralUsesThePseudolocaleBands(t *testing.T) {
	src := &message.Message{
		Key:  "a.b",
		Kind: message.KindPlural,
		Plural: &message.Plural{
			Argument: "count",
			Forms: map[message.Category]string{
				message.CatOne:   "{count} file",
				message.CatFew:   "{count} files",
				message.CatMany:  "{count} files",
				message.CatOther: "{count} files",
			},
		},
	}

	got, err := pseudo.Message(src)
	if err != nil {
		t.Fatal(err)
	}
	if got.Locale != pseudo.Locale {
		t.Fatalf("locale = %q", got.Locale)
	}

	cats := got.Plural.Categories()
	if len(cats) != 2 {
		t.Fatalf("categories = %v, want the pseudolocale's two", cats)
	}
	if _, ok := got.Plural.Forms[message.CatFew]; ok {
		t.Fatal("a Polish band leaked into a two-band locale")
	}
	if !strings.Contains(got.Plural.Forms[message.CatOther], "{count}") {
		t.Fatalf("placeholder lost: %q", got.Plural.Forms[message.CatOther])
	}
}

func TestZeroBandIsCarriedThrough(t *testing.T) {
	src := &message.Message{
		Kind: message.KindPlural,
		Plural: &message.Plural{
			Argument: "count",
			Forms: map[message.Category]string{
				message.CatZero:  "No files",
				message.CatOne:   "{count} file",
				message.CatOther: "{count} files",
			},
		},
	}

	got, err := pseudo.Message(src)
	if err != nil {
		t.Fatal(err)
	}
	zero, ok := got.Plural.Forms[message.CatZero]
	if !ok {
		t.Fatal("the zero band was dropped")
	}
	if !strings.HasPrefix(zero, "[") {
		t.Fatalf("zero band not pseudolocalised: %q", zero)
	}
}

// A source with fewer bands than the pseudolocale needs must still produce a
// complete message.
func TestMissingBandFallsBackToOther(t *testing.T) {
	src := &message.Message{
		Kind: message.KindPlural,
		Plural: &message.Plural{
			Argument: "count",
			Forms:    map[message.Category]string{message.CatOther: "{count} files"},
		},
	}

	got, err := pseudo.Message(src)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Plural.Forms[message.CatOne]; !ok {
		t.Fatalf("no one band: %v", got.Plural.Forms)
	}
}
