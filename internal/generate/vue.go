// internal/generate/vue.go

package generate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/plural"
)

// Vue writes vue-i18n resources for one target, split by locale and namespace
// so the app can lazy-load a chunk at a time.
//
// Placeholders pass through untouched: canonical {name} is already vue-i18n's
// syntax. Plurals become a pipe string whose band order only the generated
// index function knows.
func Vue(out *Output, cfg *config.Config, set *message.Set, target message.Target) error {
	dir := string(target)

	for _, locale := range cfg.Locales {
		rule, err := plural.For(locale)
		if err != nil {
			return err
		}

		byNamespace := map[string]map[string]any{}

		for _, key := range set.Keys() {
			m, ok := set.Get(locale, key)
			if !ok || !cfg.Wants(m, target) {
				continue
			}

			text, err := vueText(m, rule)
			if err != nil {
				return err
			}

			ns := m.Namespace
			if byNamespace[ns] == nil {
				byNamespace[ns] = map[string]any{}
			}
			if err := nest(byNamespace[ns], key, text); err != nil {
				return fmt.Errorf("generate: %s: %w", key, err)
			}
		}

		for _, ns := range sortedNamespaces(byNamespace) {
			rel := fmt.Sprintf("%s/%s/%s.json", dir, locale, ns)
			if err := out.AddJSON(rel, byNamespace[ns]); err != nil {
				return err
			}
		}
	}
	return nil
}

func vueText(m *message.Message, rule plural.Rule) (string, error) {
	if m.Kind == message.KindPlain {
		return m.Value, nil
	}

	bands := rule.Bands(m.Plural)
	if len(bands) == 0 {
		return "", fmt.Errorf("plural has no usable forms for %s", m.Locale)
	}

	parts := make([]string, 0, len(bands))
	for _, c := range bands {
		text := m.Plural.Forms[c]
		if strings.Contains(text, "|") {
			// A literal pipe would split the message into another band.
			return "", fmt.Errorf("plural form %q contains a pipe, which vue-i18n reads as a band separator", c)
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " | "), nil
}

// nest turns common.action.accept into {common: {action: {accept: ...}}},
// which is the shape vue-i18n expects.
//
// The namespace segment stays in the tree: the app registers each file under
// its own name, and dropping it would make two namespaces collide at the root.
func nest(root map[string]any, key string, value string) error {
	parts := strings.Split(key, ".")

	node := root
	for i, part := range parts[:len(parts)-1] {
		child, ok := node[part]
		if !ok {
			next := map[string]any{}
			node[part] = next
			node = next
			continue
		}

		next, ok := child.(map[string]any)
		if !ok {
			return fmt.Errorf("%q is both a message and a group at %q",
				strings.Join(parts[:i+1], "."), part)
		}
		node = next
	}

	last := parts[len(parts)-1]
	if _, taken := node[last]; taken {
		return fmt.Errorf("%q is both a message and a group", key)
	}
	node[last] = value
	return nil
}

func sortedNamespaces(in map[string]map[string]any) []string {
	out := make([]string, 0, len(in))
	for ns := range in {
		out = append(out, ns)
	}
	sort.Strings(out)
	return out
}
