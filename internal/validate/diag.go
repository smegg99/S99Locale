// internal/validate/diag.go

// Package validate compares messages across locales.
//
// Every failure it catches is invisible at runtime: a missing translation
// renders the key, a wrong placeholder renders nothing, and a missing plural
// band renders the wrong case.
package validate

import (
	"sort"

	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
)

type Severity string

const (
	SevError   Severity = "error"
	SevWarning Severity = "warning"
)

// Codes. Stable, because they end up in suppression comments and CI output.
//
// 001, 002 and 004 are raised during parsing and forwarded by Run; they
// are declared in parse so there is one definition of each.
const (
	CodeYAML           = parse.CodeYAML
	CodeSchema         = parse.CodeSchema
	CodePlaceholder    = "I18N003" // placeholder mismatch across locales
	CodeDuplicate      = parse.CodeDuplicate
	CodeMissingKey     = "I18N005" // key absent from a locale
	CodeEmpty          = "I18N007" // empty translation
	CodeKindMismatch   = "I18N008" // plain in one locale, plural in another
	CodePluralMissing  = "I18N009" // required category absent
	CodePluralExtra    = "I18N010" // category the locale does not have
	CodePluralArgument = "I18N011" // argument differs across locales
	CodeUndeclared     = "I18N012" // placeholder used but not declared
	CodeUnused         = "I18N013" // placeholder declared but not used
	CodeTypeMismatch   = "I18N014" // placeholder type differs across locales
	CodeGoCollision    = "I18N015" // two placeholders collide once PascalCased
	CodeOverride       = "I18N016" // a later root shadowed an earlier one
	CodePluralArgType  = "I18N018" // plural argument is not an integer

	// Printed by lint's formatters, never raised as a Diagnostic, so Messages has no key for them.
	CodePossibleShared        = "I18N021" // keys that may be one message
	CodeReferencedNotDeclared = "I18N030" // a reference with no declaration
	CodeDeclaredNotReferenced = "I18N032" // a declaration with no reference
)

// Diagnostic is one finding: a message key and typed arguments, never a sentence.
type Diagnostic struct {
	Code     string   `json:"code"`
	Severity Severity `json:"severity"`
	// Message is a diag key named after the site that raised it; one code has several sites.
	Message string `json:"message"`
	// Args are the sentence's placeholders, still typed.
	Args map[string]any `json:"args,omitempty"`
	// Cause is a third-party parser's own text, kept untranslated.
	Cause  string         `json:"cause,omitempty"`
	Key    string         `json:"key,omitempty"`
	Locale string         `json:"locale,omitempty"`
	Source message.Source `json:"source"`
	// Context is rendered under the sentence as label/value pairs, in order.
	Context []Pair `json:"context,omitempty"`
}

// Pair is one context line under a diagnostic; a word travels as a key, a locale code as data.
type Pair struct {
	Label    string `json:"label,omitempty"`
	LabelKey string `json:"labelKey,omitempty"`
	Value    string `json:"value,omitempty"`
	ValueKey string `json:"valueKey,omitempty"`
}

// Report is everything one run found.
type Report struct {
	Diagnostics []Diagnostic
}

func (r *Report) add(d Diagnostic) { r.Diagnostics = append(r.Diagnostics, d) }

func (r *Report) errorAt(code, msg, key, locale string, src message.Source, args map[string]any) {
	r.add(Diagnostic{
		Code: code, Severity: SevError, Message: msg, Args: args,
		Key: key, Locale: locale, Source: src,
	})
}

func (r *Report) warnAt(code, msg, key, locale string, src message.Source, args map[string]any) {
	r.add(Diagnostic{
		Code: code, Severity: SevWarning, Message: msg, Args: args,
		Key: key, Locale: locale, Source: src,
	})
}

func (r *Report) Errors() []Diagnostic   { return r.filter(SevError) }
func (r *Report) Warnings() []Diagnostic { return r.filter(SevWarning) }

func (r *Report) filter(s Severity) []Diagnostic {
	var out []Diagnostic
	for _, d := range r.Diagnostics {
		if d.Severity == s {
			out = append(out, d)
		}
	}
	return out
}

func (r *Report) HasErrors() bool { return len(r.Errors()) > 0 }

// Sort orders diagnostics by code then key, for stable output.
func (r *Report) Sort() {
	sort.SliceStable(r.Diagnostics, func(i, j int) bool {
		a, b := r.Diagnostics[i], r.Diagnostics[j]
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		if a.Key != b.Key {
			return a.Key < b.Key
		}
		return a.Locale < b.Locale
	})
}
