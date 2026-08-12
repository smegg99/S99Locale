// internal/catalog/catalog.go

// Package catalog holds loc's own message catalogue.
package catalog

import (
	"embed"
	"io/fs"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github.com/smegg99/s99locale/lang"
)

// EnvVar overrides the OS locale for loc alone.
const EnvVar = "LOC_LANG"

//go:embed locales/*.json
var files embed.FS

var bundle = newBundle()

// newBundle globs the embedded files, so a locale added to config.cue needs no edit here.
func newBundle() *i18n.Bundle {
	// loc generates its catalogue into this package because go:embed cannot leave it.
	built := i18n.NewBundle(language.Make(DefaultLocale))

	names, err := fs.Glob(files, "locales/*.json")
	if err != nil {
		panic("catalog: glob: " + err.Error())
	}
	for _, name := range names {
		// Embedded generated files parse the same on every run, so tests catch a failure.
		if _, err := built.LoadMessageFileFS(files, name); err != nil {
			panic("catalog: load " + name + ": " + err.Error())
		}
	}
	return built
}

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
