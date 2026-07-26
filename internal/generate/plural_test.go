// internal/generate/plural_test.go

package generate_test

import (
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/generate"
)

const pluralFixture = `
reeds:
  plural:
    argument: count
    forms:
%s
  placeholders:
    count: {type: integer}
`

func pluralProject(t *testing.T) *generate.Outputs {
	t.Helper()
	en := `      one: "{count} reed"
      other: "{count} reeds"`
	pl := `      one: "{count} głos"
      few: "{count} głosy"
      many: "{count} głosów"
      other: "{count} głosów"`

	// tuner is unlisted in the config, so it reaches every target.
	_, out := project(t, map[string]string{
		"en/tuner.yaml": strings.Replace(pluralFixture, "%s", en, 1),
		"pl/tuner.yaml": strings.Replace(pluralFixture, "%s", pl, 1),
	})
	return out
}

// Polish gets four bands in the order the generated index function expects.
func TestVuePluralBandOrder(t *testing.T) {
	out := pluralProject(t)

	pl := jsonFile(t, out, "web/pl/tuner.json")
	got := pl["tuner"].(map[string]any)["reeds"].(string)

	want := "{count} głos | {count} głosy | {count} głosów | {count} głosów"
	if got != want {
		t.Fatalf("pipe string =\n  %q\nwant\n  %q", got, want)
	}

	en := jsonFile(t, out, "web/en/tuner.json")
	gotEn := en["tuner"].(map[string]any)["reeds"].(string)
	if gotEn != "{count} reed | {count} reeds" {
		t.Fatalf("en = %q", gotEn)
	}
}

func TestGeneratedPluralsFileCoversEveryLocale(t *testing.T) {
	_, out := project(t, fixture)
	src := string(file(t, out, "web/plurals.gen.ts"))

	for _, want := range []string{"plural_en", "plural_pl", "pluralRules"} {
		if !strings.Contains(src, want) {
			t.Fatalf("plurals.gen.ts missing %q", want)
		}
	}

	// Each locale carries its own band order; Polish has four bands.
	if !strings.Contains(src, "const cats_pl = ['one', 'few', 'many', 'other']") {
		t.Fatalf("no Polish band order:\n%s", src)
	}
	if !strings.Contains(src, "const cats_en = ['one', 'other']") {
		t.Fatalf("no English band order:\n%s", src)
	}
	// Selection uses CLDR, as go-i18n does.
	if !strings.Contains(src, "new Intl.PluralRules(tag)") {
		t.Fatal("the index function does not select through Intl.PluralRules")
	}
}

func TestGoCodeIsTyped(t *testing.T) {
	out := pluralProject(t)
	src := string(file(t, out, "backend/messages_gen.go"))

	if !strings.Contains(src, "type TunerReedsParams struct") {
		t.Fatalf("no typed params:\n%s", src)
	}
	if !strings.Contains(src, "Count int") {
		t.Fatal("the count parameter is not typed as an int")
	}
}
