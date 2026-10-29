// lang/lang_test.go

package lang_test

import (
	"strings"
	"testing"

	"github.com/smegg99/s99locale/lang"
)

// env turns a table row into a LookupEnv, so tests never mutate the process environment.
func env(pairs ...string) func(string) (string, bool) {
	set := map[string]string{}
	for _, pair := range pairs {
		name, value, _ := strings.Cut(pair, "=")
		set[name] = value
	}
	return func(name string) (string, bool) {
		value, ok := set[name]
		return value, ok
	}
}

func TestDetect(t *testing.T) {
	for _, c := range []struct {
		env  []string
		want string
	}{
		{nil, "en"},
		{[]string{"LOC_LANG=pl"}, "pl"},
		{[]string{"LOC_LANG=en", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LANG=pl_PL.UTF-8"}, "pl"},
		{[]string{"LC_ALL=en_US.UTF-8", "LC_MESSAGES=pl_PL.UTF-8", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LC_MESSAGES=pl_PL.UTF-8", "LANG=en_US.UTF-8"}, "pl"},
		{[]string{"LC_ALL=C", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LC_ALL=POSIX", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LC_ALL=C.UTF-8", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LC_ALL=C.utf8", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LOC_LANG=C", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LC_MESSAGES=de_DE.UTF-8", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LOC_LANG=de", "LANG=pl_PL.UTF-8"}, "en"},
		{[]string{"LANG=pl@euro"}, "pl"},
		{[]string{"LANG=pl-PL"}, "pl"},
		{[]string{"LANG=PL"}, "pl"},
		{[]string{"LC_ALL=", "LANG=pl_PL.UTF-8"}, "pl"},
		{[]string{"LC_ALL=   ", "LANG=pl_PL.UTF-8"}, "pl"},
		{[]string{"LANGUAGE=pl:en"}, "en"},
	} {
		got := lang.Detect(lang.Options{
			Var:       "LOC_LANG",
			Supported: []string{"en", "pl"},
			Default:   "en",
			LookupEnv: env(c.env...),
		})
		if got != c.want {
			t.Errorf("Detect(%v) = %q, want %q", c.env, got, c.want)
		}
	}
}

// LOC_LANG=de falls back silently, while an explicit --lang de is a usage error in cmd/loc.

// TestDetectReadsTheProcessEnvironment covers the nil LookupEnv path real callers take.
func TestDetectReadsTheProcessEnvironment(t *testing.T) {
	t.Setenv("LOC_LANG", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "pl_PL.UTF-8")

	got := lang.Detect(lang.Options{
		Var:       "LOC_LANG",
		Supported: []string{"en", "pl"},
		Default:   "en",
	})
	if got != "pl" {
		t.Errorf("Detect = %q, want pl", got)
	}
}

// LANGUAGE alone names pl, so a Detect that read LANGUAGE would return pl.
func TestDetectIgnoresLANGUAGE(t *testing.T) {
	for _, pairs := range [][]string{
		{"LANGUAGE=pl", "LANG=en_US.UTF-8"},
		{"LANGUAGE=pl"},
	} {
		got := lang.Detect(lang.Options{
			Var:       "LOC_LANG",
			Supported: []string{"en", "pl"},
			Default:   "en",
			LookupEnv: env(pairs...),
		})
		if got != "en" {
			t.Errorf("Detect(%v) = %q, want en", pairs, got)
		}
	}
}

// Default is listed second, so a Detect that returned Supported[0] would give pl.
func TestDetectFallsBackToDefault(t *testing.T) {
	for _, pairs := range [][]string{
		nil,
		{"LANG=de_DE.UTF-8"},
		{"LC_ALL=C", "LANG=pl_PL.UTF-8"},
	} {
		got := lang.Detect(lang.Options{
			Var:       "LOC_LANG",
			Supported: []string{"pl", "en"},
			Default:   "en",
			LookupEnv: env(pairs...),
		})
		if got != "en" {
			t.Errorf("Detect(%v) = %q, want en", pairs, got)
		}
	}
}

// LANG is read against the region-qualified tags loc's config accepts.
func TestDetectSelectsARegionalCatalogue(t *testing.T) {
	for _, c := range []struct {
		env  []string
		want string
	}{
		{[]string{"LANG=pl_PL.UTF-8"}, "pl-PL"},
		{[]string{"LANG=en_GB.UTF-8"}, "en-US"},
		{[]string{"LANG=de_DE.UTF-8"}, "en-US"},
	} {
		got := lang.Detect(lang.Options{
			Var:       "LOC_LANG",
			Supported: []string{"en-US", "pl-PL"},
			Default:   "en-US",
			LookupEnv: env(c.env...),
		})
		if got != c.want {
			t.Errorf("Detect(%v) = %q, want %q", c.env, got, c.want)
		}
	}
}

func TestMatch(t *testing.T) {
	supported := []string{"en", "pl"}

	for _, c := range []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"pl", "pl", true},
		{"PL", "pl", true},
		{"pl_PL.UTF-8", "pl", true},
		{"pl-PL", "pl", true},
		{"pl@euro", "pl", true},
		{"C", "", false},
		{"POSIX", "", false},
		{"C.UTF-8", "", false},
		{"de", "", false},
		{"", "", false},
	} {
		got, ok := lang.Match(supported, c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("Match(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.wantOK)
		}
	}
}

// Match returns the entry's own spelling: same region, then bare language, then first match.
func TestMatchRegion(t *testing.T) {
	for _, c := range []struct {
		supported []string
		in        string
		want      string
		wantOK    bool
	}{
		{[]string{"en-US", "pl-PL"}, "pl-PL", "pl-PL", true},
		{[]string{"en-US", "pl-PL"}, "pl_PL.UTF-8", "pl-PL", true},
		{[]string{"en-US", "pl-PL"}, "pl", "pl-PL", true},
		{[]string{"en-US", "pl-PL"}, "de_DE.UTF-8", "", false},
		{[]string{"en-US", "pl-PL"}, "C.UTF-8", "", false},
		{[]string{"en", "pt-BR", "pt-PT"}, "pt_PT.UTF-8", "pt-PT", true},
		{[]string{"en", "pt-BR", "pt-PT"}, "pt_BR", "pt-BR", true},
		{[]string{"en", "pt-BR", "pt-PT"}, "PT-pt", "pt-PT", true},
		{[]string{"en", "pt-BR", "pt-PT"}, "pt", "pt-BR", true},
		{[]string{"en", "pt", "pt-BR"}, "pt_BR.UTF-8", "pt-BR", true},
		{[]string{"en", "pt-BR", "pt"}, "pt_PT.UTF-8", "pt", true},
		{[]string{"en", "pt_BR", "pt_PT"}, "pt-PT", "pt_PT", true},
		{[]string{"zh-Hant-TW", "zh-Hant-HK"}, "zh-Hant-HK", "zh-Hant-HK", true},
		{[]string{"en", "pl"}, "pl_PL.UTF-8", "pl", true},
	} {
		got, ok := lang.Match(c.supported, c.in)
		if got != c.want || ok != c.wantOK {
			t.Errorf("Match(%v, %q) = (%q, %v), want (%q, %v)", c.supported, c.in, got, ok, c.want, c.wantOK)
		}
	}
}
