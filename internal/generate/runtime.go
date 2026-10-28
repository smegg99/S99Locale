// internal/generate/runtime.go

package generate

// runtimeSource is emitted into messages_gen.go so the generated file imports only go-i18n.
const runtimeSource = `
func plain(l *i18n.Localizer, id string) string {
	out, err := l.Localize(&i18n.LocalizeConfig{MessageID: id})
	if err != nil {
		// The raw key is visibly wrong; an empty string reads as a blank label.
		return id
	}
	return out
}

func withData(l *i18n.Localizer, id string, data map[string]any) string {
	out, err := l.Localize(&i18n.LocalizeConfig{MessageID: id, TemplateData: data})
	if err != nil {
		return id
	}
	return out
}

func withCount(l *i18n.Localizer, id string, count int, data map[string]any) string {
	out, err := l.Localize(&i18n.LocalizeConfig{
		MessageID:    id,
		TemplateData: data,
		PluralCount:  count,
	})
	if err != nil {
		return id
	}
	return out
}
`

// embedSource is emitted into embed_gen.go; the files are generated, so a load failure is a build bug.
const embedSource = `import (
	"embed"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var LocaleFiles embed.FS

// NewBundle loads locales/<locale>.json from LocaleFiles for every locale in Locales, DefaultLocale first.
func NewBundle() *i18n.Bundle {
	bundle := i18n.NewBundle(language.Make(DefaultLocale))
	for _, locale := range Locales {
		if _, err := bundle.LoadMessageFileFS(LocaleFiles, "locales/"+locale+".json"); err != nil {
			panic("load embedded locale " + locale + ": " + err.Error())
		}
	}
	return bundle
}
`
