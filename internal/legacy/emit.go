// internal/legacy/emit.go

package legacy

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/message"
)

// emit writes one namespace as canonical YAML.
//
// Hand-written rather than marshalled: key order has to be stable, the header
// comment has to survive, and a marshaller would quote every scalar.
func emit(msgs map[string]*message.Message, namespace string) ([]byte, error) {
	var b bytes.Buffer

	keys := make([]string, 0, len(msgs))
	for k := range msgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// A tree, so nested keys nest rather than repeating their prefix.
	root := map[string]any{}
	for _, key := range keys {
		trimmed := strings.TrimPrefix(key, namespace+".")
		if err := insert(root, strings.Split(trimmed, "."), msgs[key]); err != nil {
			return nil, fmt.Errorf("legacy: %s: %w", key, err)
		}
	}

	fmt.Fprintf(&b, "# %s.yaml\n#\n# Imported by `loc import`. Descriptions were not recoverable from the\n# source bundles and are worth adding.\n\n", namespace)
	writeNode(&b, root, 0)

	return b.Bytes(), nil
}

func insert(node map[string]any, path []string, m *message.Message) error {
	if len(path) == 1 {
		if _, taken := node[path[0]]; taken {
			return fmt.Errorf("%q is both a message and a group", path[0])
		}
		node[path[0]] = m
		return nil
	}

	child, ok := node[path[0]]
	if !ok {
		next := map[string]any{}
		node[path[0]] = next
		return insert(next, path[1:], m)
	}

	next, ok := child.(map[string]any)
	if !ok {
		return fmt.Errorf("%q is both a message and a group", path[0])
	}
	return insert(next, path[1:], m)
}

func writeNode(b *bytes.Buffer, node map[string]any, depth int) {
	pad := strings.Repeat("  ", depth)

	names := make([]string, 0, len(node))
	for k := range node {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, name := range names {
		switch v := node[name].(type) {
		case *message.Message:
			fmt.Fprintf(b, "%s%s:\n", pad, name)
			writeMessage(b, v, depth+1)
		case map[string]any:
			fmt.Fprintf(b, "%s%s:\n", pad, name)
			writeNode(b, v, depth+1)
		}
	}
}

func writeMessage(b *bytes.Buffer, m *message.Message, depth int) {
	pad := strings.Repeat("  ", depth)

	if m.Kind == message.KindPlain {
		fmt.Fprintf(b, "%svalue: %s\n", pad, quote(m.Value))
	} else {
		fmt.Fprintf(b, "%splural:\n", pad)
		fmt.Fprintf(b, "%s  argument: %s\n", pad, m.Plural.Argument)
		fmt.Fprintf(b, "%s  forms:\n", pad)
		for _, c := range m.Plural.Categories() {
			fmt.Fprintf(b, "%s    %s: %s\n", pad, c, quote(m.Plural.Forms[c]))
		}
	}

	if len(m.Placeholders) == 0 {
		return
	}
	fmt.Fprintf(b, "%splaceholders:\n", pad)
	for _, p := range message.SortedPlaceholders(m.Placeholders) {
		fmt.Fprintf(b, "%s  %s: {type: %s}\n", pad, p.Name, p.Type)
	}
}

// quote always double-quotes, so a value starting with a brace, a colon or a
// digit cannot change meaning on the way in.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')

	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}

	b.WriteByte('"')
	return b.String()
}
