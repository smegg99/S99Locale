// internal/parse/build.go

package parse

import (
	"fmt"

	"github.com/smegg99/s99locale/internal/message"
)

// buildMessage turns one YAML node into a Message; path locates the node's line in its file.
func buildMessage(node map[string]any, key, path, locale, file string, lines lineFinder) (*message.Message, *Error) {
	src := lines.of(path, file)

	m := &message.Message{
		Key:          key,
		Locale:       locale,
		Namespace:    message.Namespace(key),
		Placeholders: map[string]message.Placeholder{},
		Source:       src,
	}

	fail := func(code, msg string, cause error) *Error {
		e := &Error{Code: code, Source: src, Key: key, Message: msg}
		if cause != nil {
			e.Cause = cause.Error()
		}
		return e
	}

	if raw, ok := node["value"]; ok {
		if _, both := node["plural"]; both {
			return nil, fail(CodeSchema, "valueAndPlural", nil)
		}
		text, ok := raw.(string)
		if !ok {
			return nil, fail(CodeSchema, "valueNotString", nil)
		}
		m.Kind = message.KindPlain
		m.Value = text
	} else {
		p, err := buildPlural(node["plural"])
		if err != nil {
			return nil, fail(CodeSchema, "badPlural", err)
		}
		m.Kind = message.KindPlural
		m.Plural = p
	}

	if raw, ok := node["description"]; ok {
		text, ok := raw.(string)
		if !ok {
			return nil, fail(CodeSchema, "descriptionNotString", nil)
		}
		m.Description = text
	}

	if raw, ok := node["placeholders"]; ok {
		ph, err := buildPlaceholders(raw)
		if err != nil {
			return nil, fail(CodeSchema, "badPlaceholders", err)
		}
		m.Placeholders = ph
	}

	return m, nil
}

func buildPlural(raw any) (*message.Plural, error) {
	node, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("`plural` must be a mapping with `argument` and `forms`")
	}

	arg, ok := node["argument"].(string)
	if !ok || arg == "" {
		return nil, fmt.Errorf("`plural.argument` is required and names the counted placeholder")
	}

	forms, ok := node["forms"].(map[string]any)
	if !ok || len(forms) == 0 {
		return nil, fmt.Errorf("`plural.forms` is required")
	}

	p := &message.Plural{Argument: arg, Forms: map[message.Category]string{}}
	for _, name := range sortedMapKeys(forms) {
		cat, err := message.ParseCategory(name)
		if err != nil {
			return nil, err
		}
		text, ok := forms[name].(string)
		if !ok {
			return nil, fmt.Errorf("plural form %q must be a string", name)
		}
		p.Forms[cat] = text
	}
	return p, nil
}

func buildPlaceholders(raw any) (map[string]message.Placeholder, error) {
	node, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("`placeholders` must be a mapping")
	}

	out := map[string]message.Placeholder{}
	for _, name := range sortedMapKeys(node) {
		spec, ok := node[name].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("placeholder %q must be a mapping, such as {type: string}", name)
		}
		text, ok := spec["type"].(string)
		if !ok {
			return nil, fmt.Errorf("placeholder %q needs a `type`", name)
		}
		kind, err := message.ParsePlaceholderType(text)
		if err != nil {
			return nil, fmt.Errorf("placeholder %q: %w", name, err)
		}
		out[name] = message.Placeholder{Name: name, Type: kind}
	}
	return out, nil
}
