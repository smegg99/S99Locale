// internal/catalog/catalog.go

// Package catalog holds loc's own message catalogue.
package catalog

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/smegg99/s99locale/lang"
)

// EnvVar overrides the OS locale for loc alone.
const EnvVar = "LOC_LANG"

var bundle = NewBundle()

// Localizer renders in one locale, falling back to the default.
func Localizer(locale string) *i18n.Localizer {
	if locale == "" {
		locale = DefaultLocale
	}
	return i18n.NewLocalizer(bundle, locale, DefaultLocale)
}

// FromEnv is the language loc renders in when no --lang was given.
func FromEnv() string {
	return lang.Detect(lang.Options{Var: EnvVar, Supported: Locales, Default: DefaultLocale})
}

// Supported reports whether a language has a catalogue, and its canonical name.
func Supported(value string) (string, bool) { return lang.Match(Locales, value) }
