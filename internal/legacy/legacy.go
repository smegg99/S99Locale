// internal/legacy/legacy.go

// Package legacy reads existing locale trees into canonical YAML.
//
// Reads what a project already has and writes the canonical form, reporting
// everything it could not work out rather than guessing.
package legacy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/plural"
	"github.com/smegg99/s99locale/internal/validate"
)

// Note is something a human has to finish. Descriptions cannot be recovered
// from a bundle, and some plural shapes are genuinely ambiguous.
type Note struct {
	Key    string
	Locale string
	Detail string
}

// Result is one import run.
type Result struct {
	// Files maps a canonical path (en/common.yaml) to its contents.
	Files map[string][]byte
	Notes []Note
	Count int
}

func (r *Result) note(key, locale, format string, args ...any) {
	r.Notes = append(r.Notes, Note{Key: key, Locale: locale, Detail: fmt.Sprintf(format, args...)})
}

// Sources are the trees to read. Either may be empty.
type Sources struct {
	// Go is a directory of flat go-i18n files, one per locale: en.json.
	Go string
	// Vue is a directory of nested vue-i18n files: en/common.json.
	Vue string
}

// Import reads both trees and produces canonical YAML.
//
// A key present in both is taken once. The two trees disagreeing about its
// text is exactly the drift being migrated away from, so it is reported.
func Import(src Sources, locales []string) (*Result, error) {
	res := &Result{Files: map[string][]byte{}}

	// locale -> namespace -> key -> message
	byLocale := map[string]map[string]map[string]*message.Message{}
	for _, l := range locales {
		byLocale[l] = map[string]map[string]*message.Message{}
	}

	if src.Go != "" {
		if err := readGo(res, byLocale, src.Go, locales); err != nil {
			return nil, err
		}
	}
	if src.Vue != "" {
		if err := readVue(res, byLocale, src.Vue, locales); err != nil {
			return nil, err
		}
	}

	for _, locale := range locales {
		foldZeroBands(byLocale[locale])

		for _, ns := range sortedNamespaces(byLocale[locale]) {
			body, err := emit(byLocale[locale][ns], ns)
			if err != nil {
				return nil, err
			}
			res.Files[locale+"/"+ns+".yaml"] = body
			res.Count += len(byLocale[locale][ns])
		}
	}
	return res, nil
}

// record files a message, reporting a conflict rather than picking a winner.
func record(res *Result, into map[string]map[string]*message.Message, m *message.Message) {
	ns := m.Namespace
	if into[ns] == nil {
		into[ns] = map[string]*message.Message{}
	}

	if prev, ok := into[ns][m.Key]; ok {
		if !sameText(prev, m) {
			res.note(m.Key, m.Locale,
				"the two source trees disagree about this string; the first was kept")
		}
		return
	}
	into[ns][m.Key] = m
}

// foldZeroBands puts a standalone `key.zero` back inside `key`.
//
// The Go emitter writes the opt-in zero band as its own message, because no
// CLDR rule would select it from inside a plural. Read back naively it becomes
// a child of the plural it belongs to, which is not a shape canonical YAML
// can express.
//
// Only folded when the parent exists and is genuinely a plural: a message
// legitimately named "zero" must survive untouched.
func foldZeroBands(byNamespace map[string]map[string]*message.Message) {
	for _, msgs := range byNamespace {
		for key, m := range msgs {
			parentKey, ok := strings.CutSuffix(key, ".zero")
			if !ok || m.Kind != message.KindPlain {
				continue
			}

			parent, ok := msgs[parentKey]
			if !ok || parent.Kind != message.KindPlural {
				continue
			}

			parent.Plural.Forms[message.CatZero] = m.Value
			delete(msgs, key)
		}
	}
}

func sameText(a, b *message.Message) bool {
	at, bt := a.Text(), b.Text()
	if len(at) != len(bt) {
		return false
	}
	for i := range at {
		if at[i] != bt[i] {
			return false
		}
	}
	return true
}

// splitPlural turns a vue-i18n pipe string into categories.
//
// The band count is the only signal, so the mapping is by position: a locale
// wanting four categories with five bands has an opt-in zero at the front.
// Fewer bands than the locale requires is the clamping case, and it is
// genuinely ambiguous - that gets reported rather than guessed.
func splitPlural(res *Result, key, locale, text string) *message.Plural {
	parts := strings.Split(text, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	rule, err := plural.For(locale)
	if err != nil {
		res.note(key, locale, "no plural rule for this locale")
		return nil
	}

	required := rule.Required
	var cats []message.Category

	switch {
	case len(parts) == len(required):
		cats = required
	case len(parts) == len(required)+1:
		cats = append([]message.Category{message.CatZero}, required...)
	case len(parts) < len(required):
		res.note(key, locale,
			"%d bands for a locale that needs %d (%s); the missing ones need writing",
			len(parts), len(required), names(required))
		// Map what is there, from the end: the last band is always "other".
		cats = required[len(required)-len(parts):]
	default:
		res.note(key, locale, "%d bands is more than %s allows", len(parts), locale)
		return nil
	}

	forms := map[message.Category]string{}
	for i, c := range cats {
		forms[c] = parts[i]
	}

	arg, ok := pluralArgument(parts)
	if !ok {
		res.note(key, locale, "could not tell which placeholder is the count; assumed {count}")
		arg = "count"
	}
	return &message.Plural{Argument: arg, Forms: forms}
}

// pluralArgument picks the counted placeholder.
//
// One placeholder across every band is unambiguous. More than one needs a
// convention, and anything else is a guess the caller should not trust.
func pluralArgument(parts []string) (string, bool) {
	seen := map[string]int{}
	for _, p := range parts {
		for _, name := range validate.Placeholders(p) {
			seen[name]++
		}
	}

	if len(seen) == 1 {
		for name := range seen {
			return name, true
		}
	}
	for _, common := range []string{"count", "n", "num", "total"} {
		if _, ok := seen[common]; ok {
			return common, true
		}
	}
	return "", false
}

func names(cats []message.Category) string {
	out := make([]string, 0, len(cats))
	for _, c := range cats {
		out = append(out, string(c))
	}
	return strings.Join(out, ", ")
}

func sortedNamespaces(in map[string]map[string]*message.Message) []string {
	out := make([]string, 0, len(in))
	for ns := range in {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}
