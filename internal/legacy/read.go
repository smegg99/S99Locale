// internal/legacy/read.go

package legacy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/validate"
)

// readGo reads flat go-i18n bundles: one file per locale, dotted keys, and
// {{.Name}} interpolation.
func readGo(res *Result, into map[string]map[string]map[string]*message.Message, dir string, locales []string) error {
	for _, locale := range locales {
		path := filepath.Join(dir, locale+".json")

		raw, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("legacy: read %s: %w", path, err)
		}

		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("legacy: parse %s: %w", path, err)
		}

		for _, key := range sortedAnyKeys(doc) {
			m := goMessage(res, key, locale, doc[key])
			if m != nil {
				record(res, into[locale], m)
			}
		}
	}
	return nil
}

func goMessage(res *Result, key, locale string, node any) *message.Message {
	m := &message.Message{
		Key:          key,
		Locale:       locale,
		Namespace:    message.Namespace(key),
		Placeholders: map[string]message.Placeholder{},
	}

	switch v := node.(type) {
	case string:
		// A go-i18n zero band is written as its own key by loc's own emitter.
		// Folding it back into the plural is handled after both trees are read.
		m.Kind = message.KindPlain
		m.Value = fromGoTemplate(v)
	case map[string]any:
		p := &message.Plural{Forms: map[message.Category]string{}}
		for _, name := range sortedAnyKeys(v) {
			cat, err := message.ParseCategory(name)
			if err != nil {
				res.note(key, locale, "unknown plural category %q", name)
				return nil
			}
			text, ok := v[name].(string)
			if !ok {
				res.note(key, locale, "plural form %q is not a string", name)
				return nil
			}
			p.Forms[cat] = fromGoTemplate(text)
		}

		arg, ok := pluralArgument(formTexts(p))
		if !ok {
			res.note(key, locale, "could not tell which placeholder is the count; assumed {count}")
			arg = "count"
		}
		p.Argument = arg

		m.Kind = message.KindPlural
		m.Plural = p
	default:
		res.note(key, locale, "unsupported value")
		return nil
	}

	inferPlaceholders(m)
	return m
}

// readVue reads nested vue-i18n bundles: locale/namespace.json, {name}
// interpolation, pipe-separated plural bands.
func readVue(res *Result, into map[string]map[string]map[string]*message.Message, dir string, locales []string) error {
	for _, locale := range locales {
		localeDir := filepath.Join(dir, locale)

		entries, err := os.ReadDir(localeDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("legacy: read %s: %w", localeDir, err)
		}

		var files []string
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
				files = append(files, e.Name())
			}
		}
		sort.Strings(files)

		for _, name := range files {
			path := filepath.Join(localeDir, name)

			raw, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("legacy: read %s: %w", path, err)
			}

			var doc map[string]any
			if err := json.Unmarshal(raw, &doc); err != nil {
				return fmt.Errorf("legacy: parse %s: %w", path, err)
			}

			// A vue bundle may or may not already be wrapped in its namespace.
			// Both shapes exist in the wild, so the file name decides.
			ns := strings.TrimSuffix(name, filepath.Ext(name))
			flat := map[string]string{}
			flatten(doc, "", flat)

			for _, key := range sortedStringKeys(flat) {
				full := key
				if !strings.HasPrefix(key, ns+".") && key != ns {
					full = ns + "." + key
				}
				m := vueMessage(res, full, locale, flat[key])
				if m != nil {
					record(res, into[locale], m)
				}
			}
		}
	}
	return nil
}

func vueMessage(res *Result, key, locale, text string) *message.Message {
	m := &message.Message{
		Key:          key,
		Locale:       locale,
		Namespace:    message.Namespace(key),
		Placeholders: map[string]message.Placeholder{},
	}

	if strings.Contains(text, "|") {
		p := splitPlural(res, key, locale, text)
		if p == nil {
			return nil
		}
		m.Kind = message.KindPlural
		m.Plural = p
	} else {
		m.Kind = message.KindPlain
		m.Value = text
	}

	inferPlaceholders(m)
	return m
}

// inferPlaceholders derives declarations from the text. Types are a guess: the
// counted argument is an integer, everything else a string.
func inferPlaceholders(m *message.Message) {
	seen := map[string]struct{}{}
	for _, text := range m.Text() {
		for _, name := range validate.Placeholders(text) {
			seen[name] = struct{}{}
		}
	}
	if m.Kind == message.KindPlural {
		seen[m.Plural.Argument] = struct{}{}
	}

	for name := range seen {
		kind := message.TypeString
		if m.Kind == message.KindPlural && name == m.Plural.Argument {
			kind = message.TypeInteger
		}
		m.Placeholders[name] = message.Placeholder{Name: name, Type: kind}
	}
}

// fromGoTemplate rewrites {{.Name}} back to {name}.
func fromGoTemplate(text string) string {
	var b strings.Builder

	for i := 0; i < len(text); i++ {
		if !strings.HasPrefix(text[i:], "{{.") {
			b.WriteByte(text[i])
			continue
		}
		end := strings.Index(text[i:], "}}")
		if end < 0 {
			b.WriteByte(text[i])
			continue
		}

		field := text[i+3 : i+end]
		if field == "" || !isIdent(field) {
			b.WriteString(text[i : i+end+2])
			i += end + 1
			continue
		}
		b.WriteString("{" + lowerFirst(field) + "}")
		i += end + 1
	}
	return b.String()
}

func isIdent(s string) bool {
	for i, r := range s {
		switch {
		case unicode.IsLetter(r), r == '_':
		case unicode.IsDigit(r) && i > 0:
		default:
			return false
		}
	}
	return s != ""
}

// lowerFirst undoes the PascalCase the Go emitter applied. It cannot recover
// an underscore, so user_id comes back as userId - which is a better name
// anyway, and validation would have rejected the pair as colliding.
func lowerFirst(s string) string {
	r := []rune(s)
	r[0] = unicode.ToLower(r[0])
	return string(r)
}

// flatten turns a nested vue bundle into dotted keys.
func flatten(node map[string]any, prefix string, out map[string]string) {
	for _, k := range sortedAnyKeys(node) {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}

		switch v := node[k].(type) {
		case string:
			out[key] = v
		case map[string]any:
			flatten(v, key, out)
		}
	}
}

func formTexts(p *message.Plural) []string {
	out := make([]string, 0, len(p.Forms))
	for _, c := range p.Categories() {
		out = append(out, p.Forms[c])
	}
	return out
}

func sortedAnyKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedStringKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
