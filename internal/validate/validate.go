// internal/validate/validate.go

package validate

import (
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/plural"
)

// Run checks a parsed set against the config.
func Run(cfg *config.Config, res *parse.Result) *Report {
	r := &Report{}

	for _, e := range res.Errors {
		r.add(Diagnostic{
			Code: e.Code, Severity: SevError, Message: e.Message, Args: e.Args,
			Cause: e.Cause, Key: e.Key, Source: e.Source,
		})
	}
	for _, o := range res.Overrides {
		r.add(Diagnostic{
			Code: CodeOverride, Severity: SevWarning, Message: "override",
			Key: o.Key, Locale: o.Locale, Source: o.To,
			Context: []Pair{
				{LabelKey: "labelWas", Value: o.From.String()},
				{LabelKey: "labelNow", Value: o.To.String()},
			},
		})
	}

	base := cfg.DefaultLocale
	for _, key := range res.Set.Keys() {
		checkKey(r, cfg, res.Set, key, base)
	}
	r.Sort()
	return r
}

func checkKey(r *Report, cfg *config.Config, set *message.Set, key, base string) {
	ref, hasRef := set.Get(base, key)
	found, from := ref, base
	if !hasRef {
		found, from = firstHolder(cfg.Locales, set, key)
	}

	for _, locale := range cfg.Locales {
		m, ok := set.Get(locale, key)
		if !ok {
			src := message.Source{}
			if found != nil {
				src = found.Source
			}
			r.errorAt(CodeMissingKey, "missingKey", key, locale, src,
				map[string]any{"locale": locale, "base": from})
			continue
		}
		checkOne(r, cfg, m)

		if hasRef && locale != base {
			compare(r, ref, m, base)
		}
	}
}

// firstHolder is the first configured locale that defines key, for a key the default locale lacks.
func firstHolder(locales []string, set *message.Set, key string) (*message.Message, string) {
	for _, locale := range locales {
		if m, ok := set.Get(locale, key); ok {
			return m, locale
		}
	}
	return nil, ""
}

// checkOne is everything decidable from a single message.
func checkOne(r *Report, cfg *config.Config, m *message.Message) {
	rule, err := plural.For(m.Locale)
	if err != nil {
		r.add(Diagnostic{
			Code: CodePluralMissing, Severity: SevError, Message: "noPluralRule",
			Cause: err.Error(), Key: m.Key, Locale: m.Locale, Source: m.Source,
		})
		return
	}

	for _, text := range m.Text() {
		if strings.TrimSpace(text) == "" {
			r.errorAt(CodeEmpty, "emptyTranslation", m.Key, m.Locale, m.Source, nil)
		}
	}

	if m.Kind == message.KindPlural {
		checkPlural(r, m, rule)
	}
	checkPlaceholders(r, m)
}

func checkPlural(r *Report, m *message.Message, rule plural.Rule) {
	for _, c := range rule.Missing(m.Plural) {
		r.errorAt(CodePluralMissing, "pluralFormMissing", m.Key, m.Locale, m.Source,
			map[string]any{"locale": m.Locale, "category": string(c)})
	}
	for _, c := range rule.Extra(m.Plural) {
		r.errorAt(CodePluralExtra, "pluralCategoryExtra", m.Key, m.Locale, m.Source,
			map[string]any{"locale": m.Locale, "category": string(c)})
	}

	if _, ok := m.Plural.Forms[message.CatOther]; !ok {
		r.errorAt(CodePluralMissing, "pluralOtherMissing", m.Key, m.Locale, m.Source, nil)
	}
	if m.Plural.Argument == "" {
		r.errorAt(CodePluralArgument, "pluralArgumentMissing", m.Key, m.Locale, m.Source, nil)
		return
	}

	// Generated Go passes this to go-i18n's int PluralCount; another type does not compile.
	if p, ok := m.Placeholders[m.Plural.Argument]; ok && p.Type != message.TypeInteger {
		r.errorAt(CodePluralArgType, "pluralArgumentType", m.Key, m.Locale, m.Source,
			map[string]any{"name": p.Name, "kind": string(p.Type)})
	}
}

func checkPlaceholders(r *Report, m *message.Message) {
	used := map[string]struct{}{}
	for _, text := range m.Text() {
		for _, name := range Placeholders(text) {
			used[name] = struct{}{}
		}
	}

	if m.Kind == message.KindPlural && m.Plural.Argument != "" {
		// The count argument must be declared even when a form does not interpolate it.
		used[m.Plural.Argument] = struct{}{}
	}

	for _, name := range sortedSet(used) {
		if _, ok := m.Placeholders[name]; !ok {
			r.errorAt(CodeUndeclared, "placeholderUndeclared", m.Key, m.Locale, m.Source,
				map[string]any{"name": name})
		}
	}
	for _, p := range message.SortedPlaceholders(m.Placeholders) {
		if _, ok := used[p.Name]; !ok {
			r.warnAt(CodeUnused, "placeholderUnused", m.Key, m.Locale, m.Source,
				map[string]any{"name": p.Name})
		}
	}

	checkGoCollisions(r, m)
}

// checkGoCollisions catches names that differ only in casing and become one Go template field.
func checkGoCollisions(r *Report, m *message.Message) {
	seen := map[string]string{}
	for _, p := range message.SortedPlaceholders(m.Placeholders) {
		field := GoField(p.Name)
		if other, dup := seen[field]; dup {
			r.errorAt(CodeGoCollision, "goFieldCollision", m.Key, m.Locale, m.Source,
				map[string]any{"first": other, "second": p.Name, "field": field})
			continue
		}
		seen[field] = p.Name
	}
}
