// internal/config/config.go

// Package config loads and validates i18n/config.cue.
package config

import (
	"context"
	_ "embed"
	"fmt"
	"maps"
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
}

type Config struct {
	DefaultLocale  string                    `json:"defaultLocale"`
	Locales        []string                  `json:"locales"`
	Targets        []message.Target          `json:"targets"`
	Roots          []string                  `json:"roots"`
	Output         string                    `json:"output"`
	Outputs        map[message.Target]string `json:"outputs"`
	Namespaces     map[string]Namespace      `json:"namespaces"`
	DefaultTargets []message.Target          `json:"defaultTargets"`
	DynamicKeys    []string                  `json:"dynamicKeys"`
	Scan           []string                  `json:"scan"`
	KeyPatterns    []string                  `json:"keyPatterns"`
	Go             Go                        `json:"go"`

	// Dir is the config's directory; every relative path resolves against it.
	Dir string `json:"-"`
}

// Load reads and validates a config file.
func Load(ctx context.Context, path string) (*Config, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cueCtx := cuecontext.New()

	compiled := cueCtx.CompileBytes(schema, cue.Filename("config.cue"))
	if err := compiled.Err(); err != nil {
		return nil, fmt.Errorf("config: schema: %w", err)
	}

	instance, err := loadFile(cueCtx, path)
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

	for _, t := range c.DefaultTargets {
		if !slices.Contains(c.Targets, t) {
			return fmt.Errorf("defaultTargets names %q, which is not in targets %v", t, c.Targets)
		}
	}
	for name, ns := range c.Namespaces {
		for _, t := range ns.Targets {
			if !slices.Contains(c.Targets, t) {
				return fmt.Errorf("namespace %q targets %q, which is not in targets %v", name, t, c.Targets)
			}
		}
	}

	// Nothing reads the redirect of a target outside targets, so naming one is an error.
	for _, t := range slices.Sorted(maps.Keys(c.Outputs)) {
		if !slices.Contains(c.Targets, t) {
			return fmt.Errorf("outputs names %q, which is not in targets %v", t, c.Targets)
		}
	}
	return c.targetDirs()
}

// targetDirs rejects targets that share a directory or hold a canonical source root.
func (c *Config) targetDirs() error {
	// clean runs unattended, so a bad output path is rejected here.
	base, err := filepath.Abs(c.Dir)
	if err != nil {
		return fmt.Errorf("resolve the config directory: %w", err)
	}

	// Compare absolute paths: filepath.Rel cannot relate a relative -c to an absolute entry.
	resolved := *c
	resolved.Dir = base
	sources := append(resolved.AbsRoots(), resolved.AbsScan()...)

	// loc prunes the output parent, so it must sit below the config directory.
	out := resolved.AbsOutput()
	if out == base || !Under(base, out) {
		return fmt.Errorf("output %q resolves to %s, which is not below the config directory", c.Output, out)
	}
	for _, root := range sources {
		if Under(out, root) {
			return fmt.Errorf("output %q resolves to %s, which holds the source root %s", c.Output, out, root)
		}
	}

	type placed struct {
		target message.Target
		dir    string
	}
	var seen []placed

	for _, t := range c.Targets {
		dir := resolved.AbsTargetOutput(t)

		if dir == base {
			return fmt.Errorf("target %q writes into the config directory", t)
		}
		if !c.Redirected(t) && !Under(base, dir) {
			return fmt.Errorf("target %q writes to %s, which is outside the config directory", t, dir)
		}
		for _, root := range sources {
			if Under(dir, root) {
				return fmt.Errorf("target %q writes into %s, which holds the source root %s", t, dir, root)
			}
		}
		// loc prunes the output tree, so a redirected target must stay out of it.
		if c.Redirected(t) && Under(out, dir) {
			return fmt.Errorf("target %q writes into %s, which is inside the output tree %s", t, dir, out)
		}
		for _, prev := range seen {
			if Under(dir, prev.dir) || Under(prev.dir, dir) {
				return fmt.Errorf("targets %q and %q both write into %s", prev.target, t, dir)
			}
		}
		seen = append(seen, placed{target: t, dir: dir})
	}
	return nil
}

// Under reports whether child is at or beneath parent.
func Under(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
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

// Redirected reports whether a target has its own directory in `outputs`.
func (c *Config) Redirected(t message.Target) bool {
	dir, ok := c.Outputs[t]
	return ok && dir != ""
}

// AbsTargetOutput resolves where one target's files are written.
func (c *Config) AbsTargetOutput(t message.Target) string {
	if c.Redirected(t) {
		return c.abs(c.Outputs[t])
	}
	return c.DefaultTargetOutput(t)
}

// DefaultTargetOutput is <output>/<target>, used when outputs does not redirect a target.
func (c *Config) DefaultTargetOutput(t message.Target) string {
	return filepath.Join(c.AbsOutput(), string(t))
}

// AbsTargetOutputs resolves every configured target's directory, in Targets order.
func (c *Config) AbsTargetOutputs() []string {
	out := make([]string, 0, len(c.Targets))
	for _, t := range c.Targets {
		out = append(out, c.AbsTargetOutput(t))
	}
	return out
}

// TargetLabel is a target's directory as a user would type it, for messages.
func (c *Config) TargetLabel(t message.Target) string {
	dir := c.AbsTargetOutput(t)

	rel, err := filepath.Rel(c.Dir, dir)
	if err != nil {
		return filepath.ToSlash(dir)
	}
	return filepath.ToSlash(rel)
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
		return filepath.Clean(p)
	}
	return filepath.Join(c.Dir, p)
}

// TargetsFor returns the targets a namespace reaches: its own, else defaultTargets, else every target.
func (c *Config) TargetsFor(namespace string) []message.Target {
	ns, ok := c.Namespaces[namespace]
	if !ok || len(ns.Targets) == 0 {
		if len(c.DefaultTargets) > 0 {
			ns.Targets = c.DefaultTargets
		} else {
			return slices.Clone(c.Targets)
		}
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
