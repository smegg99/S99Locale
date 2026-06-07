// internal/parse/parse.go

// Package parse turns canonical YAML into the neutral AST.
//
// A message that does not fit the schema becomes a diagnostic, never a
// silently dropped key.
package parse

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/smegg99/s99locale/internal/message"
)

// Result carries what parsed and what did not; parsing continues past a bad file.
type Result struct {
	Set       *message.Set
	Overrides []Override
	Errors    []Error
}

// Override records a later root shadowing an earlier one.
type Override struct {
	Key    string
	Locale string
	From   message.Source
	To     message.Source
}

// Codes raised here; validate re-exports them in the full registry.
const (
	CodeYAML      = "I18N001" // invalid structure
	CodeSchema    = "I18N002" // message does not fit the schema
	CodeDuplicate = "I18N004" // duplicate fully qualified key
	CodeTarget    = "I18N006" // unknown or wrong target
)

type Error struct {
	Code   string
	Source message.Source
	Key    string
	Detail string
}

func (e Error) Error() string {
	if e.Key == "" {
		return fmt.Sprintf("%s %s: %s", e.Code, e.Source, e.Detail)
	}
	return fmt.Sprintf("%s %s: %s: %s", e.Code, e.Source, e.Key, e.Detail)
}

// Roots parses every root in order. Later roots override earlier ones.
//
// ctx is checked per file rather than per line: a locale file parses in
// microseconds, so finer granularity buys nothing but noise.
func Roots(ctx context.Context, roots []string, locales []string) (*Result, error) {
	res := &Result{Set: message.NewSet(locales)}

	for i, root := range roots {
		if err := parseRoot(ctx, res, root, i, locales); err != nil {
			return nil, err
		}
	}
	return res, nil
}

func parseRoot(ctx context.Context, res *Result, root string, index int, locales []string) error {
	for _, locale := range locales {
		dir := filepath.Join(root, locale)

		info, err := os.Stat(dir)
		if os.IsNotExist(err) {
			// A root need not carry every locale; validation reports missing translations.
			continue
		}
		if err != nil {
			return fmt.Errorf("parse: stat %s: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("parse: %s is not a directory", dir)
		}

		files, err := yamlFiles(dir)
		if err != nil {
			return err
		}
		for _, file := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			parseFile(res, dir, file, locale, index)
		}
	}
	return nil
}

// yamlFiles walks a locale directory in sorted order.
func yamlFiles(dir string) ([]string, error) {
	var out []string

	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if ext := filepath.Ext(p); ext == ".yaml" || ext == ".yml" {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse: walk %s: %w", dir, err)
	}
	sort.Strings(out)
	return out, nil
}

// prefix derives the key prefix from the path: en/auth/login.yaml -> auth.login
func prefix(dir, file string) (string, error) {
	rel, err := filepath.Rel(dir, file)
	if err != nil {
		return "", err
	}
	rel = strings.TrimSuffix(rel, filepath.Ext(rel))

	parts := strings.Split(filepath.ToSlash(rel), "/")
	return strings.Join(parts, "."), nil
}

func parseFile(res *Result, dir, file, locale string, root int) {
	if info, err := os.Stat(file); err == nil && info.Size() > MaxFileSize {
		res.Errors = append(res.Errors, Error{
			Code: CodeYAML, Source: message.Source{File: file},
			Detail: fmt.Sprintf("file is %d MB; the limit is %d MB. Split it into smaller files",
				info.Size()>>20, MaxFileSize>>20),
		})
		return
	}

	raw, err := os.ReadFile(file)
	if err != nil {
		res.Errors = append(res.Errors, Error{
			Code: CodeYAML, Source: message.Source{File: file},
			Detail: err.Error(),
		})
		return
	}

	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		res.Errors = append(res.Errors, Error{
			Code: CodeYAML, Source: message.Source{File: file},
			Detail: "invalid YAML: " + err.Error(),
		})
		return
	}

	pre, err := prefix(dir, file)
	if err != nil {
		res.Errors = append(res.Errors, Error{
			Code: CodeYAML, Source: message.Source{File: file}, Detail: err.Error(),
		})
		return
	}

	lines := lineIndex(raw)
	walk(res, doc, pre, "", file, locale, root, lines)
}

// walk descends the document. A node holding "value" or "plural" is a message;
// anything else is another level of key.
//
// key is the fully qualified name, which carries the file's own prefix; path is
// where the node sits inside this file, which is what the line index knows.
func walk(res *Result, node map[string]any, prefix, path, file, locale string, root int, lines lineFinder) {
	for _, name := range sortedMapKeys(node) {
		child, ok := node[name].(map[string]any)
		key := joinKey(prefix, name)
		here := joinKey(path, name)

		if !ok {
			res.Errors = append(res.Errors, Error{
				Code: CodeSchema, Source: lines.of(here, file), Key: key,
				Detail: "expected a message or a group, got a bare value. Wrap it as `value: \"...\"`",
			})
			continue
		}

		if isMessage(child) {
			m, err := buildMessage(child, key, here, locale, file, lines)
			if err != nil {
				res.Errors = append(res.Errors, *err)
				continue
			}
			m.Root = root
			record(res, m, locale)
			continue
		}
		walk(res, child, key, here, file, locale, root, lines)
	}
}

func record(res *Result, m *message.Message, locale string) {
	if prev, ok := res.Set.Get(locale, m.Key); ok {
		if prev.Root != m.Root {
			res.Overrides = append(res.Overrides, Override{
				Key: m.Key, Locale: locale, From: prev.Source, To: m.Source,
			})
		} else {
			res.Errors = append(res.Errors, Error{
				Code: CodeDuplicate, Source: m.Source, Key: m.Key,
				Detail: "duplicate key, already defined at " + prev.Source.String(),
			})
			return
		}
	}
	res.Set.Messages[locale][m.Key] = m
}

func isMessage(node map[string]any) bool {
	_, plain := node["value"]
	_, plural := node["plural"]
	return plain || plural
}

func joinKey(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "." + name
}

func sortedMapKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
