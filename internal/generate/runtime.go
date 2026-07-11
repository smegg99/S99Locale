// internal/generate/runtime.go

package generate

// runtimeSource is the helper block emitted into messages_gen.go.
//
// Generated rather than shipped as an importable package, so a consuming app
// depends on go-i18n alone and never on loc.
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
