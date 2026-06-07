// internal/generate/runtime.go

package generate

// runtimeSource is the helper block emitted into messages_gen.go.
//
// Generated rather than shipped as an importable package, so a consuming app
// depends on go-i18n alone and never on loc.
const runtimeSource = `
// zeroKey is the sub-key the opt-in zero band is written under.
//
// go-i18n selects by CLDR rule, and no rule returns "zero" for the locales we
// target: Polish resolves a count of 0 to Many. So the zero band is a separate
// message, reached only by the branch in the accessors below.
const zeroKey = ".zero"

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

func zeroForm(l *i18n.Localizer, id string, data map[string]any) string {
	out, err := l.Localize(&i18n.LocalizeConfig{MessageID: id + zeroKey, TemplateData: data})
	if err != nil {
		return withCount(l, id, 0, data)
	}
	return out
}
`
