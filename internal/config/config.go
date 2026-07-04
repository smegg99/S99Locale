// internal/config/config.go

// Package config loads i18n/config.cue.
//
// CUE validates the shape, so an unknown target or a missing locale fails at
// load rather than halfway through a build with half the files written.
package config

import (
	_ "embed"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	cueerrors "cuelang.org/go/cue/errors"

	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/plural"
)

//go:embed config.cue
var schema []byte

type Namespace struct {
	Targets []message.Target `json:"targets"`
}

type Go struct {
	Package string `json:"package"`
	Output  string `json:"output"`
}

type Config struct {
	DefaultLocale string               `json:"defaultLocale"`
	Locales       []string             `json:"locales"`
	Targets       []message.Target     `json:"targets"`
	Roots         []string             `json:"roots"`
	Output        string               `json:"output"`
	Namespaces    map[string]Namespace `json:"namespaces"`
	DynamicKeys   []string             `json:"dynamicKeys"`
	Scan          []string             `json:"scan"`
	KeyPatterns   []string             `json:"keyPatterns"`
	Go            Go                   `json:"go"`

	// Dir is the config's directory; every relative path resolves against it.
	Dir string `json:"-"`
}

// Load reads and validates a config file.
func Load(path string) (*Config, error) {
	ctx := cuecontext.New()

	compiled := ctx.CompileBytes(schema, cue.Filename("config.cue"))
	if err := compiled.Err(); err != nil {
		return nil, fmt.Errorf("config: schema: %w", err)
	}

	instance, err := loadFile(ctx, path)
	if err != nil {
		return nil, err
	}

	unified := compiled.LookupPath(cue.ParsePath("#Config")).Unify(instance)
	if err := unified.Validate(cue.Concrete(true)); err != nil {
		// Details names the field and value; the plain disjunction error names neither.
		return nil, fmt.Errorf("config: %s:\n%s", path, cueerrors.Details(err, nil))
	}

	var cfg Config
	if err := unified.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config: decode %s: %w", path, err)
	}

	cfg.Dir = filepath.Dir(path)
	if err := cfg.check(); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	return &cfg, nil
}

// check covers what CUE cannot: cross-field agreement and the plural table.
func (c *Config) check() error {
	if !slices.Contains(c.Locales, c.DefaultLocale) {
		return fmt.Errorf("defaultLocale %q is not in locales %v", c.DefaultLocale, c.Locales)
	}

	for _, locale := range c.Locales {
		if !plural.Known(locale) {
			return fmt.Errorf("no plural rule for locale %q; known locales are %s",
				locale, strings.Join(plural.Locales(), ", "))
		}
	}

	seen := map[string]struct{}{}
	for _, locale := range c.Locales {
		if _, dup := seen[locale]; dup {
			return fmt.Errorf("locale %q listed twice", locale)
		}
		seen[locale] = struct{}{}
	}

	for name, ns := range c.Namespaces {
		for _, t := range ns.Targets {
			if !slices.Contains(c.Targets, t) {
				return fmt.Errorf("namespace %q targets %q, which is not in targets %v", name, t, c.Targets)
			}
		}
	}
	return nil
}

// AbsRoots resolves the canonical roots against the config's directory.
func (c *Config) AbsRoots() []string {
	out := make([]string, 0, len(c.Roots))
	for _, r := range c.Roots {
		out = append(out, c.abs(r))
	}
	return out
}

func (c *Config) AbsOutput() string { return c.abs(c.Output) }

// AbsGoOutput resolves where the Go target is written, or "" when it stays
// under AbsOutput with everything else.
func (c *Config) AbsGoOutput() string {
	if c.Go.Output == "" {
		return ""
	}
	return c.abs(c.Go.Output)
}

// AbsScan resolves the scan roots against the config's directory.
func (c *Config) AbsScan() []string {
	out := make([]string, 0, len(c.Scan))
	for _, r := range c.Scan {
		out = append(out, c.abs(r))
	}
	return out
}

func (c *Config) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(c.Dir, p)
}

// TargetsFor returns the targets a namespace reaches.
// A namespace with no entry reaches every target.
func (c *Config) TargetsFor(namespace string) []message.Target {
	ns, ok := c.Namespaces[namespace]
	if !ok || len(ns.Targets) == 0 {
		return slices.Clone(c.Targets)
	}

	out := make([]message.Target, 0, len(ns.Targets))
	for _, t := range c.Targets {
		if slices.Contains(ns.Targets, t) {
			out = append(out, t)
		}
	}
	return out
}

// Wants reports whether a message reaches a target.
func (c *Config) Wants(m *message.Message, target message.Target) bool {
	return slices.Contains(c.TargetsFor(m.Namespace), target)
}
