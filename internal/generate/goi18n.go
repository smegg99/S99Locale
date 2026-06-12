// internal/generate/goi18n.go

package generate

import (
	"fmt"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/plural"
	"github.com/smegg99/s99locale/internal/validate"
)

// GoBundle writes one flat go-i18n file per locale.
//
// go-i18n interpolates with text/template, so canonical {name} becomes
// {{.Name}} here rather than in the source, which also has to reach vue-i18n.
func GoBundle(out *Output, cfg *config.Config, set *message.Set) error {
	for _, locale := range cfg.BuildLocales() {
		rule, err := plural.For(locale)
		if err != nil {
			return err
		}

		bundle := map[string]any{}
		for _, key := range set.Keys() {
			m, ok := set.Get(locale, key)
			if !ok || !cfg.Wants(m, message.TargetBackend) {
				continue
			}

			entry, err := goEntry(m, rule)
			if err != nil {
				return fmt.Errorf("generate: %s: %w", key, err)
			}
			bundle[key] = entry

			// The zero band goes out as its own message. Inside the plural it
			// would be dead: no CLDR rule returns "zero" for these locales, so
			// only the generated accessor's count==0 branch can reach it.
			if m.Kind == message.KindPlural {
				if text, ok := m.Plural.Forms[message.CatZero]; ok {
					bundle[key+".zero"] = goTemplate(text)
				}
			}
		}

		if err := out.AddJSON(goLayout(cfg).bundle(locale), bundle); err != nil {
			return err
		}
	}
	return nil
}

// goEntry is a string for a plain message, or a category map for a plural.
func goEntry(m *message.Message, rule plural.Rule) (any, error) {
	if m.Kind == message.KindPlain {
		return goTemplate(m.Value), nil
	}

	forms := map[string]any{}
	for _, c := range rule.Bands(m.Plural) {
		if c == message.CatZero {
			continue // written separately; see GoBundle
		}
		forms[string(c)] = goTemplate(m.Plural.Forms[c])
	}
	if len(forms) == 0 {
		return nil, fmt.Errorf("plural has no usable forms for %s", m.Locale)
	}
	return forms, nil
}

// goTemplate rewrites {name} to {{.Name}}, leaving {{ }} alone.
func goTemplate(text string) string {
	var b strings.Builder

	for i := 0; i < len(text); i++ {
		if text[i] != '{' {
			b.WriteByte(text[i])
			continue
		}
		// A doubled brace is Go template syntax or an escaped brace; leave it.
		if i+1 < len(text) && text[i+1] == '{' {
			b.WriteString("{{")
			i++
			continue
		}

		end := strings.IndexByte(text[i:], '}')
		if end < 0 {
			b.WriteByte(text[i])
			continue
		}

		name := text[i+1 : i+end]
		field := validate.GoField(name)
		if field == "" || !isPlaceholder(name) {
			b.WriteString(text[i : i+end+1])
			i += end
			continue
		}

		b.WriteString("{{." + field + "}}")
		i += end
	}
	return b.String()
}

func isPlaceholder(name string) bool {
	names := validate.Placeholders("{" + name + "}")
	return len(names) == 1 && names[0] == name
}
