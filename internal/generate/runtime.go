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
