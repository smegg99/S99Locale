// internal/generate/gocode.go

package generate

import (
	"fmt"
	"go/format"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/validate"
)

// GoCode writes typed accessors over go-i18n, so a misspelled placeholder is a compile error.
func GoCode(out *Output, cfg *config.Config, set *message.Set) error {
	if cfg.Go.Package == "" {
		return nil
	}

	msgs := backendMessages(cfg, set)

	names, err := funcNames(msgs)
	if err != nil {
		return err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\npackage %s\n\n", message.GeneratedHeader, cfg.Go.Package)
	b.WriteString("import \"github.com/nicksnyder/go-i18n/v2/i18n\"\n\n")
	b.WriteString(runtimeSource)

	for _, m := range msgs {
		b.WriteString(accessor(m, names[m.Key]))
	}

	src, err := gofmt(b.String())
	if err != nil {
		return err
	}
	out.Add("messages_gen.go", src)

	return keysFile(out, cfg, msgs, names)
}

func keysFile(out *Output, cfg *config.Config, msgs []*message.Message, names map[string]string) error {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n\npackage %s\n\n", message.GeneratedHeader, cfg.Go.Package)
	b.WriteString("// Keys, for call sites that need the string rather than the accessor.\nconst (\n")
	for _, m := range msgs {
		fmt.Fprintf(&b, "\tKey%s = %q\n", names[m.Key], m.Key)
	}
	b.WriteString(")\n\n")

	fmt.Fprintf(&b, "// DefaultLocale is the locale a missing translation falls back to.\nconst DefaultLocale = %q\n\n",
		cfg.DefaultLocale)
	b.WriteString("// Locales are every locale this catalogue was built for, in config order.\nvar Locales = []string{\n")
	for _, locale := range cfg.Locales {
		fmt.Fprintf(&b, "\t%q,\n", locale)
	}
	b.WriteString("}\n")

	src, err := gofmt(b.String())
	if err != nil {
		return err
	}
	out.Add("keys_gen.go", src)
	return nil
}

// gofmt canonicalises the emitted source, so `gofmt -w .` does not make it stale.
func gofmt(src string) ([]byte, error) {
	out, err := format.Source([]byte(src))
	if err != nil {
		return nil, fmt.Errorf("generate: emitted invalid Go: %w", err)
	}
	return out, nil
}

// backendMessages is every message the backend renders, in key order.
func backendMessages(cfg *config.Config, set *message.Set) []*message.Message {
	var out []*message.Message

	for _, key := range set.Keys() {
		m, ok := set.Get(cfg.DefaultLocale, key)
		if !ok || !cfg.Wants(m, message.TargetBackend) {
			continue
		}
		out = append(out, m)
	}
	return out
}

// fixedNames are the package-level identifiers every catalogue emits, owned by no key.
var fixedNames = map[string]string{"DefaultLocale": "the DefaultLocale constant", "Locales": "the Locales list"}

// funcNames maps keys to exported Go identifiers and rejects any two emitted identifiers that are equal:
// the accessor, its Key constant, its Params type and the package's fixed names.
func funcNames(msgs []*message.Message) (map[string]string, error) {
	out := map[string]string{}
	owner := map[string]string{}
	for name, what := range fixedNames {
		owner[name] = what
	}
	claim := func(ident, by string) error {
		if other, dup := owner[ident]; dup {
			return fmt.Errorf("generate: %s and %s both generate %s", other, by, ident)
		}
		owner[ident] = by
		return nil
	}

	for _, m := range msgs {
		name := exported(m.Key)
		by := fmt.Sprintf("%q", m.Key)
		idents := []string{name, "Key" + name}
		if len(m.Placeholders) > 0 {
			idents = append(idents, name+"Params")
		}
		for _, ident := range idents {
			if err := claim(ident, by); err != nil {
				return nil, err
			}
		}
		out[m.Key] = name
	}
	return out, nil
}

// exported turns auth.login.failed into AuthLoginFailed.
func exported(key string) string {
	var b strings.Builder
	for _, part := range strings.Split(key, ".") {
		b.WriteString(validate.GoField(part))
	}
	return b.String()
}

func accessor(m *message.Message, name string) string {
	var b strings.Builder

	params := message.SortedPlaceholders(m.Placeholders)

	// One line: a description is free text, and a line break in it would end the comment.
	doc := fmt.Sprintf("// %s renders %s.\n", name, m.Key)
	if text := strings.Join(strings.Fields(m.Description), " "); text != "" {
		doc = fmt.Sprintf("// %s %s\n", name, text)
	}

	if len(params) == 0 {
		b.WriteString("\n" + doc)
		fmt.Fprintf(&b, "func %s(l *i18n.Localizer) string {\n", name)
		fmt.Fprintf(&b, "\treturn plain(l, %q)\n}\n", m.Key)
		return b.String()
	}

	fmt.Fprintf(&b, "\n// %sParams are the placeholders of %s.\n", name, m.Key)
	fmt.Fprintf(&b, "type %sParams struct {\n", name)
	for _, p := range params {
		fmt.Fprintf(&b, "\t%s %s\n", validate.GoField(p.Name), goType(p.Type))
	}
	b.WriteString("}\n\n")

	b.WriteString(doc)
	fmt.Fprintf(&b, "func %s(l *i18n.Localizer, p %sParams) string {\n", name, name)
	b.WriteString("\tdata := map[string]any{\n")
	for _, p := range params {
		fmt.Fprintf(&b, "\t\t%q: p.%s,\n", validate.GoField(p.Name), validate.GoField(p.Name))
	}
	b.WriteString("\t}\n")

	if m.Kind != message.KindPlural {
		fmt.Fprintf(&b, "\treturn withData(l, %q, data)\n}\n", m.Key)
		return b.String()
	}

	arg := validate.GoField(m.Plural.Argument)
	fmt.Fprintf(&b, "\treturn withCount(l, %q, p.%s, data)\n}\n", m.Key, arg)

	return b.String()
}

func goType(t message.PlaceholderType) string {
	switch t {
	case message.TypeInteger:
		return "int"
	case message.TypeNumber:
		return "float64"
	default:
		return "string"
	}
}

// GoNames maps a generated identifier back to the key it renders, for the source scan.
func GoNames(cfg *config.Config, set *message.Set) map[string]string {
	msgs := backendMessages(cfg, set)

	names, err := funcNames(msgs)
	if err != nil {
		// Generation reports a collision; the scan finds nothing instead of failing.
		return map[string]string{}
	}

	out := make(map[string]string, len(names))
	for key, name := range names {
		out[name] = key
	}
	return out
}
