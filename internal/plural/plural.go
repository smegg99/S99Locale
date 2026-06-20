// internal/plural/plural.go

// Package plural knows which categories a locale actually has.
//
// Polish counts in four and vue-i18n counts in two, so a wrong band here is
// wrong in every counted string and nothing at runtime notices: the count
// reads as the wrong case, the way "2 entrys" reads in English.
package plural

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/message"
)

// Rule is what a locale requires and what it may optionally carry.
type Rule struct {
	// Required categories, in band order; every message in this locale declares all of them.
	Required []message.Category
}

// rules holds CLDR cardinal categories per language.
//
// Taken from CLDR, cross-checked against go-i18n v2.6.1's generated rules so
// the two halves of a build cannot disagree about what "few" means. Which band
// a count selects is not decided here - see generate.Plurals.
//
// A language whose CLDR rules include a real `zero` category cannot go in this
// table as it stands: ar, cy and lv all have one, and `zero` is reserved here
// for the opt-in band that no rule selects. Adding one would give a message two
// different meanings for the same name. TestNoLocaleRequiresZero holds the line.
var rules = map[string]Rule{
	"en": {Required: []message.Category{message.CatOne, message.CatOther}},
	"pl": {Required: []message.Category{message.CatOne, message.CatFew, message.CatMany, message.CatOther}},
	"de": {Required: []message.Category{message.CatOne, message.CatOther}},
	"es": {Required: []message.Category{message.CatOne, message.CatOther}},
	"fr": {Required: []message.Category{message.CatOne, message.CatOther}},
	"it": {Required: []message.Category{message.CatOne, message.CatOther}},
	"nl": {Required: []message.Category{message.CatOne, message.CatOther}},
	"pt": {Required: []message.Category{message.CatOne, message.CatOther}},
	"ru": {Required: []message.Category{message.CatOne, message.CatFew, message.CatMany, message.CatOther}},
	"uk": {Required: []message.Category{message.CatOne, message.CatFew, message.CatMany, message.CatOther}},
	"cs": {Required: []message.Category{message.CatOne, message.CatFew, message.CatMany, message.CatOther}},
	"sk": {Required: []message.Category{message.CatOne, message.CatFew, message.CatMany, message.CatOther}},
	"ja": {Required: []message.Category{message.CatOther}},
	"zh": {Required: []message.Category{message.CatOther}},
	"ko": {Required: []message.Category{message.CatOther}},
	"tr": {Required: []message.Category{message.CatOne, message.CatOther}},

	// Languages with a dual. The band order follows CategoryOrder, so `two`
	// sits between `one` and `few` in every generated pipe string.
	"sl": {Required: []message.Category{message.CatOne, message.CatTwo, message.CatFew, message.CatOther}},
	"ga": {Required: []message.Category{message.CatOne, message.CatTwo, message.CatFew, message.CatMany, message.CatOther}},
	"he": {Required: []message.Category{message.CatOne, message.CatTwo, message.CatOther}},
}

// Base strips a region: pl-PL and pl share plural rules.
func Base(locale string) string {
	if i := strings.IndexAny(locale, "-_"); i >= 0 {
		return locale[:i]
	}
	return locale
}

// For returns the rule for a locale.
func For(locale string) (Rule, error) {
	r, ok := rules[Base(locale)]
	if !ok {
		return Rule{}, fmt.Errorf("no plural rule for locale %q", locale)
	}
	return r, nil
}

// Known reports whether a locale has a rule.
func Known(locale string) bool {
	_, ok := rules[Base(locale)]
	return ok
}

// Locales returns every locale in the table, sorted.
func Locales() []string {
	out := make([]string, 0, len(rules))
	for l := range rules {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// Allowed is Required plus zero.
//
// Zero is an opt-in extra band, not a CLDR category. No targeted locale's rule
// ever selects it - Polish resolves a count of 0 to Many - so it exists only
// because "brak głosów" reads better than "0 głosów", and only generated code
// branches on it.
func (r Rule) Allowed() []message.Category {
	return append([]message.Category{message.CatZero}, r.Required...)
}

func (r Rule) Requires(c message.Category) bool {
	for _, want := range r.Required {
		if want == c {
			return true
		}
	}
	return false
}

func (r Rule) Allows(c message.Category) bool {
	return c == message.CatZero || r.Requires(c)
}

// Missing returns required categories a message did not declare.
func (r Rule) Missing(p *message.Plural) []message.Category {
	var out []message.Category
	for _, c := range r.Required {
		if _, ok := p.Forms[c]; !ok {
			out = append(out, c)
		}
	}
	return out
}

// Extra returns declared categories this locale does not have.
func (r Rule) Extra(p *message.Plural) []message.Category {
	var out []message.Category
	for _, c := range p.Categories() {
		if !r.Allows(c) {
			out = append(out, c)
		}
	}
	return out
}

// Bands is the order forms are written into a vue-i18n pipe string, filtered
// to what this message declares.
//
// The order is a private contract between the generated string and the
// generated index function. Nothing hand-written depends on it.
func (r Rule) Bands(p *message.Plural) []message.Category {
	var out []message.Category
	for _, c := range message.CategoryOrder {
		if !r.Allows(c) {
			continue
		}
		if _, ok := p.Forms[c]; ok {
			out = append(out, c)
		}
	}
	return out
}
