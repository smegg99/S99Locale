// internal/generate/pseudo.go

package generate

import (
	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/pseudo"
)

// withPseudolocale returns a set with the generated locale added.
//
// Derived at build time rather than committed: a checked-in pseudolocale is a
// translation nobody wrote that everybody has to keep in step.
func withPseudolocale(cfg *config.Config, set *message.Set) (*message.Set, error) {
	if !cfg.Pseudolocale {
		return set, nil
	}

	out := message.NewSet(cfg.BuildLocales())
	for _, locale := range cfg.Locales {
		for key, m := range set.Messages[locale] {
			out.Messages[locale][key] = m
		}
	}

	// Built from the default locale: it is the one guaranteed complete, and
	// padding a half-translated string would measure the wrong length.
	for _, key := range set.Keys() {
		m, ok := set.Get(cfg.DefaultLocale, key)
		if !ok {
			continue
		}

		fake, err := pseudo.Message(m)
		if err != nil {
			return nil, err
		}
		out.Messages[pseudo.Locale][key] = fake
	}
	return out, nil
}
