// internal/parse/build.go

package parse

import (
	"fmt"
	"sort"

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

	fail := func(code, detail string) *Error {
		return &Error{Code: code, Source: src, Key: key, Detail: detail}
	}

	if raw, ok := node["value"]; ok {
		if _, both := node["plural"]; both {
			return nil, fail(CodeSchema, "a message is either `value` or `plural`, never both")
		}
		text, ok := raw.(string)
		if !ok {
			return nil, fail(CodeSchema, "`value` must be a string")
		}
		m.Kind = message.KindPlain
		m.Value = text
	} else {
		p, err := buildPlural(node["plural"])
		if err != nil {
			return nil, fail(CodeSchema, err.Error())
		}
		m.Kind = message.KindPlural
		m.Plural = p
	}

	if raw, ok := node["description"]; ok {
		text, ok := raw.(string)
		if !ok {
			return nil, fail(CodeSchema, "`description` must be a string")
		}
		m.Description = text
	}

	if raw, ok := node["deprecated"]; ok {
		flag, ok := raw.(bool)
		if !ok {
			return nil, fail(CodeSchema, "`deprecated` must be a boolean")
		}
		m.Deprecated = flag
	}

	if raw, ok := node["placeholders"]; ok {
		ph, err := buildPlaceholders(raw)
		if err != nil {
			return nil, fail(CodeSchema, err.Error())
		}
		m.Placeholders = ph
	}

	if raw, ok := node["targets"]; ok {
		targets, err := buildTargets(raw)
		if err != nil {
			return nil, fail(CodeTarget, err.Error())
		}
		m.Targets = targets
	}

	if raw, ok := node["sharing"]; ok {
		sharing, err := buildSharing(raw)
		if err != nil {
			return nil, fail(CodeSchema, err.Error())
		}
		m.Sharing = sharing
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

func buildTargets(raw any) ([]message.Target, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("`targets` must be a list")
	}

	var out []message.Target
	for _, item := range list {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("`targets` entries must be strings")
		}
		t, err := message.ParseTarget(text)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func buildSharing(raw any) (message.Sharing, error) {
	node, ok := raw.(map[string]any)
	if !ok {
		return message.Sharing{}, fmt.Errorf("`sharing` must be a mapping")
	}

	var s message.Sharing
	if v, ok := node["isolate"]; ok {
		flag, ok := v.(bool)
		if !ok {
			return s, fmt.Errorf("`sharing.isolate` must be a boolean")
		}
		s.Isolate = flag
	}
	if v, ok := node["reason"]; ok {
		text, ok := v.(string)
		if !ok {
			return s, fmt.Errorf("`sharing.reason` must be a string")
		}
		s.Reason = text
	}
	return s, nil
}
