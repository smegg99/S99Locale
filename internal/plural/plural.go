// internal/plural/plural.go

// Package plural knows which CLDR plural categories each locale has.
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

// rules holds the CLDR cardinal categories per language, matching go-i18n v2.6.1.
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

func (r Rule) Requires(c message.Category) bool {
	for _, want := range r.Required {
		if want == c {
			return true
		}
	}
	return false
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
		if !r.Requires(c) {
			out = append(out, c)
		}
	}
	return out
}

// Bands is the pipe-string form order, filtered to the categories this message declares.
func (r Rule) Bands(p *message.Plural) []message.Category {
	var out []message.Category
	for _, c := range message.CategoryOrder {
		if !r.Requires(c) {
			continue
		}
		if _, ok := p.Forms[c]; ok {
			out = append(out, c)
		}
	}
	return out
}
