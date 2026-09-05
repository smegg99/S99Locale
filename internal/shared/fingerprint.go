// internal/shared/fingerprint.go

// Package shared finds keys whose whole contract matches across every locale.
package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/message"
)

// NormalizedPlaceholder is a placeholder stripped of anything that cannot change the output.
type NormalizedPlaceholder struct {
	Name string
	Type message.PlaceholderType
}

// NormalizedPlural is a plural's shape without its text.
type NormalizedPlural struct {
	Argument   string
	Categories []message.Category
}

// Fingerprint is a message's full contract, compared in whole and never by hash alone.
type Fingerprint struct {
	Kind         message.Kind
	Translations map[string]string
	Placeholders []NormalizedPlaceholder
	Plural       *NormalizedPlural
}

// Of builds a fingerprint from one key across every locale, or returns false when one lacks it.
func Of(set *message.Set, key string, locales []string) (Fingerprint, bool) {
	fp := Fingerprint{Translations: map[string]string{}}

	for i, locale := range locales {
		m, ok := set.Get(locale, key)
		if !ok {
			return Fingerprint{}, false
		}

		if i == 0 {
			fp.Kind = m.Kind
			fp.Placeholders = normalizePlaceholders(m)
			fp.Plural = normalizePlural(m)
		} else if m.Kind != fp.Kind {
			// Plain in one language and plural in another is not one message.
			return Fingerprint{}, false
		}

		fp.Translations[locale] = normalizeText(m)
	}
	return fp, true
}

// normalizeText joins a message's text in a stable order, trimming outer whitespace only.
func normalizeText(m *message.Message) string {
	parts := make([]string, 0, 4)
	for _, text := range m.Text() {
		clean := strings.ReplaceAll(text, "\r\n", "\n")
		parts = append(parts, strings.TrimSpace(clean))
	}
	return strings.Join(parts, "\x00")
}

func normalizePlaceholders(m *message.Message) []NormalizedPlaceholder {
	out := make([]NormalizedPlaceholder, 0, len(m.Placeholders))
	for _, p := range message.SortedPlaceholders(m.Placeholders) {
		out = append(out, NormalizedPlaceholder{Name: p.Name, Type: p.Type})
	}
	return out
}

func normalizePlural(m *message.Message) *NormalizedPlural {
	if m.Kind != message.KindPlural {
		return nil
	}
	return &NormalizedPlural{
		Argument:   m.Plural.Argument,
		Categories: m.Plural.Categories(),
	}
}

// Hash indexes a fingerprint; Equal decides equality.
func (f Fingerprint) Hash() string {
	h := sha256.New()

	fmt.Fprintf(h, "kind:%s\x00", f.Kind)
	for _, locale := range sortedKeys(f.Translations) {
		fmt.Fprintf(h, "%s=%s\x00", locale, f.Translations[locale])
	}
	for _, p := range f.Placeholders {
		fmt.Fprintf(h, "ph:%s:%s\x00", p.Name, p.Type)
	}
	if f.Plural != nil {
		fmt.Fprintf(h, "arg:%s\x00", f.Plural.Argument)
		for _, c := range f.Plural.Categories {
			fmt.Fprintf(h, "cat:%s\x00", c)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Equal compares the whole structure.
func (f Fingerprint) Equal(other Fingerprint) bool {
	if f.Kind != other.Kind {
		return false
	}
	if len(f.Translations) != len(other.Translations) {
		return false
	}
	for locale, text := range f.Translations {
		if other.Translations[locale] != text {
			return false
		}
	}

	if len(f.Placeholders) != len(other.Placeholders) {
		return false
	}
	for i, p := range f.Placeholders {
		if p != other.Placeholders[i] {
			return false
		}
	}

	if (f.Plural == nil) != (other.Plural == nil) {
		return false
	}
	if f.Plural == nil {
		return true
	}
	if f.Plural.Argument != other.Plural.Argument {
		return false
	}
	if len(f.Plural.Categories) != len(other.Plural.Categories) {
		return false
	}
	for i, c := range f.Plural.Categories {
		if c != other.Plural.Categories[i] {
			return false
		}
	}
	return true
}

// EqualIn reports whether two fingerprints agree in one given locale.
func (f Fingerprint) EqualIn(other Fingerprint, locale string) bool {
	return f.Kind == other.Kind && f.Translations[locale] == other.Translations[locale]
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
