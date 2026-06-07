// internal/validate/validate.go

package validate

import (
	"slices"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/plural"
)

// Run checks a parsed set against the config.
func Run(cfg *config.Config, res *parse.Result, strict bool) *Report {
	r := &Report{}

	for _, e := range res.Errors {
		r.errorf(e.Code, e.Key, "", e.Source, "%s", e.Detail)
	}
	for _, o := range res.Overrides {
		r.add(Diagnostic{
			Code: CodeOverride, Severity: SevWarning, Key: o.Key, Locale: o.Locale,
			Source: o.To,
			Detail: "a later root overrides an earlier definition",
			Context: []Pair{
				{Label: "was", Value: o.From.String()},
				{Label: "now", Value: o.To.String()},
			},
		})
	}

	base := cfg.DefaultLocale
	for _, key := range res.Set.Keys() {
		checkKey(r, cfg, res.Set, key, base, strict)
	}
	r.Sort()
	return r
}

func checkKey(r *Report, cfg *config.Config, set *message.Set, key, base string, strict bool) {
	ref, hasRef := set.Get(base, key)

	for _, locale := range cfg.Locales {
		m, ok := set.Get(locale, key)
		if !ok {
			src := message.Source{}
			if hasRef {
				src = ref.Source
			}
			r.errorf(CodeMissingKey, key, locale, src,
				"missing from %s; it exists in %s", locale, base)
			continue
		}
		checkOne(r, cfg, m, strict)

		if hasRef && locale != base {
			compare(r, ref, m, base)
		}
	}
}

// checkOne is everything decidable from a single message.
func checkOne(r *Report, cfg *config.Config, m *message.Message, strict bool) {
	rule, err := plural.For(m.Locale)
	if err != nil {
		r.errorf(CodePluralMissing, m.Key, m.Locale, m.Source, "%s", err.Error())
		return
	}

	for _, text := range m.Text() {
		if strings.TrimSpace(text) == "" {
			r.errorf(CodeEmpty, m.Key, m.Locale, m.Source,
				"empty translation; delete the key or translate it")
		}
	}

	if m.Kind == message.KindPlural {
		checkPlural(r, m, rule)
	}
	checkPlaceholders(r, m)
	checkTargets(r, cfg, m)

	if m.Sharing.Isolate && m.Sharing.Reason == "" && strict {
		r.errorf(CodeIsolateNoReason, m.Key, m.Locale, m.Source,
			"sharing.isolate needs a reason under --strict")
	}
}

func checkPlural(r *Report, m *message.Message, rule plural.Rule) {
	for _, c := range rule.Missing(m.Plural) {
		r.errorf(CodePluralMissing, m.Key, m.Locale, m.Source,
			"%s needs a %q form; without it that band silently renders another case",
			m.Locale, c)
	}
	for _, c := range rule.Extra(m.Plural) {
		r.errorf(CodePluralExtra, m.Key, m.Locale, m.Source,
			"%s has no %q category, so that form is unreachable", m.Locale, c)
	}

	if _, ok := m.Plural.Forms[message.CatOther]; !ok {
		r.errorf(CodePluralMissing, m.Key, m.Locale, m.Source,
			"every plural needs an \"other\" form")
	}
	if m.Plural.Argument == "" {
		r.errorf(CodePluralArgument, m.Key, m.Locale, m.Source,
			"plural.argument is required")
		return
	}

	// Generated Go passes this to go-i18n's int PluralCount; another type does not compile.
	if p, ok := m.Placeholders[m.Plural.Argument]; ok && p.Type != message.TypeInteger {
		r.errorf(CodePluralArgType, m.Key, m.Locale, m.Source,
			"plural argument %q is declared as %s; it counts, so it must be an integer",
			p.Name, p.Type)
	}
}

// checkTargets reports a message aimed at a target this build does not produce.
// Nothing else would: it is translated everywhere and reaches nothing.
func checkTargets(r *Report, cfg *config.Config, m *message.Message) {
	for _, t := range m.Targets {
		if slices.Contains(cfg.Targets, t) {
			continue
		}
		r.errorf(CodeTarget, m.Key, m.Locale, m.Source,
			"targets %q, which is not in the configured targets %v; this message reaches nothing",
			t, cfg.Targets)
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
			r.errorf(CodeUndeclared, m.Key, m.Locale, m.Source,
				"{%s} is used but not declared under placeholders", name)
		}
	}
	for _, p := range message.SortedPlaceholders(m.Placeholders) {
		if _, ok := used[p.Name]; !ok {
			r.warnf(CodeUnused, m.Key, m.Locale, m.Source,
				"placeholder %q is declared but never used", p.Name)
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
			r.errorf(CodeGoCollision, m.Key, m.Locale, m.Source,
				"{%s} and {%s} both become .%s in Go", other, p.Name, field)
			continue
		}
		seen[field] = p.Name
	}
}
