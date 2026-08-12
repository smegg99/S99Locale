// internal/catalog/catalog_test.go

package catalog_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/smegg99/s99locale/internal/catalog"
)

// Each locale renders its own text; a missing bundle would fall back to English.
func TestEveryLocaleRenders(t *testing.T) {
	for _, locale := range catalog.Locales {
		raw, err := os.ReadFile("locales/" + locale + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var values map[string]any
		if err := json.Unmarshal(raw, &values); err != nil {
			t.Fatalf("locales/%s.json: %v", locale, err)
		}
		want, ok := values[catalog.KeyCliRootShort].(string)
		if !ok || want == "" {
			t.Fatalf("locales/%s.json has no plain %s", locale, catalog.KeyCliRootShort)
		}
		if got := catalog.CliRootShort(catalog.Localizer(locale)); got != want {
			t.Errorf("%s renders rootShort as %q, want %q", locale, got, want)
		}
	}

	en := catalog.CliRootShort(catalog.Localizer("en"))
	pl := catalog.CliRootShort(catalog.Localizer("pl"))
	if en == pl {
		t.Fatal("en and pl render the same text; the Polish bundle is not loaded")
	}
}

// An empty or unknown language renders the default.
func TestLocalizerFallsBack(t *testing.T) {
	want := catalog.CliRootShort(catalog.Localizer(catalog.DefaultLocale))
	for _, locale := range []string{"", "de", "zz"} {
		if got := catalog.CliRootShort(catalog.Localizer(locale)); got != want {
			t.Errorf("Localizer(%q) renders %q, want %q", locale, got, want)
		}
	}
}

func TestSupported(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string
		ok    bool
	}{
		{"pl_PL.UTF-8", "pl", true},
		{"pl", "pl", true},
		{"en-GB", "en", true},
		{"EN", "en", true},
		{"de", "", false},
		{"C", "", false},
		{"", "", false},
	} {
		if got, ok := catalog.Supported(tc.value); got != tc.want || ok != tc.ok {
			t.Errorf("Supported(%q) = (%q, %v), want (%q, %v)", tc.value, got, ok, tc.want, tc.ok)
		}
	}
}

// LOC_LANG decides before the OS variables, and an empty one leaves the decision to them.
func TestFromEnv(t *testing.T) {
	for _, tc := range []struct {
		locLang, lcAll, lang, want string
	}{
		{"pl", "", "en_US.UTF-8", "pl"},
		{"en", "", "pl_PL.UTF-8", "en"},
		{"en", "pl_PL.UTF-8", "", "en"},
		{"", "pl_PL.UTF-8", "en_US.UTF-8", "pl"},
		{"", "", "pl_PL.UTF-8", "pl"},
		{"de", "", "pl_PL.UTF-8", catalog.DefaultLocale},
	} {
		t.Run("LOC_LANG="+tc.locLang+",LC_ALL="+tc.lcAll+",LANG="+tc.lang, func(t *testing.T) {
			t.Setenv("LC_ALL", tc.lcAll)
			t.Setenv("LC_MESSAGES", "")
			t.Setenv("LOC_LANG", tc.locLang)
			t.Setenv("LANG", tc.lang)
			if got := catalog.FromEnv(); got != tc.want {
				t.Errorf("LOC_LANG=%q LC_ALL=%q LANG=%q -> %q, want %q", tc.locLang, tc.lcAll, tc.lang, got, tc.want)
			}
		})
	}
}
