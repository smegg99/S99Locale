// internal/validate/compare.go

package validate

import (
	"slices"
	"strings"

	"github.com/smegg99/s99locale/internal/message"
)

// compare checks one locale against the default.
func compare(r *Report, ref, m *message.Message, base string) {
	if ref.Kind != m.Kind {
		r.add(Diagnostic{
			Code: CodeKindMismatch, Severity: SevError, Key: m.Key, Locale: m.Locale,
			Source: m.Source,
			Detail: "message kind differs across locales",
			Context: []Pair{
				{Label: base, Value: string(ref.Kind)},
				{Label: m.Locale, Value: string(m.Kind)},
			},
		})
		return
	}

	comparePlaceholderSets(r, ref, m, base)
	comparePlaceholderTypes(r, ref, m, base)

	if m.Kind == message.KindPlural && ref.Plural.Argument != m.Plural.Argument {
		r.add(Diagnostic{
			Code: CodePluralArgument, Severity: SevError, Key: m.Key, Locale: m.Locale,
			Source: m.Source,
			Detail: "plural argument differs across locales",
			Context: []Pair{
				{Label: base, Value: "{" + ref.Plural.Argument + "}"},
				{Label: m.Locale, Value: "{" + m.Plural.Argument + "}"},
			},
		})
	}
}

func comparePlaceholderSets(r *Report, ref, m *message.Message, base string) {
	want := declaredNames(ref)
	got := declaredNames(m)
	if slices.Equal(want, got) {
		return
	}

	r.add(Diagnostic{
		Code: CodePlaceholder, Severity: SevError, Key: m.Key, Locale: m.Locale,
		Source: m.Source,
		Detail: "placeholder mismatch",
		Context: []Pair{
			{Label: base, Value: render(want)},
			{Label: m.Locale, Value: render(got)},
		},
	})
}

func comparePlaceholderTypes(r *Report, ref, m *message.Message, base string) {
	for _, p := range message.SortedPlaceholders(ref.Placeholders) {
		other, ok := m.Placeholders[p.Name]
		if !ok || other.Type == p.Type {
			continue
		}
		r.add(Diagnostic{
			Code: CodeTypeMismatch, Severity: SevError, Key: m.Key, Locale: m.Locale,
			Source: m.Source,
			Detail: "placeholder " + p.Name + " has a different type across locales",
			Context: []Pair{
				{Label: base, Value: string(p.Type)},
				{Label: m.Locale, Value: string(other.Type)},
			},
		})
	}
}

func declaredNames(m *message.Message) []string {
	out := make([]string, 0, len(m.Placeholders))
	for _, p := range message.SortedPlaceholders(m.Placeholders) {
		out = append(out, p.Name)
	}
	return out
}

func render(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	return "{" + strings.Join(names, "} {") + "}"
}

func sortedSet(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
