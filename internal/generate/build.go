// internal/generate/build.go

package generate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"sort"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
)

// Manifest describes a build. No timestamps: they would make two identical
// builds differ.
type Manifest struct {
	SchemaVersion int                       `json:"schemaVersion"`
	SourceHash    string                    `json:"sourceHash"`
	DefaultLocale string                    `json:"defaultLocale"`
	Locales       []string                  `json:"locales"`
	Targets       map[string]TargetManifest `json:"targets"`
	Files         map[string]string         `json:"files"`
}

type TargetManifest struct {
	Namespaces []string `json:"namespaces"`
	Messages   int      `json:"messages"`
}

// Build generates every configured target into an Output, without writing.
//
// ctx is checked per target: each one is a whole bundle, which is the unit a
// cancellation can usefully land between.
func Build(ctx context.Context, cfg *config.Config, set *message.Set) (*Outputs, error) {
	out := NewOutput(cfg.AbsOutput())
	outs := &Outputs{Primary: out}
	// The Go target may be written into the package that embeds it, which loc
	// shares with hand-written source rather than owning outright.
	goOut := out
	if dir := cfg.AbsGoOutput(); dir != "" {
		goOut = NewSharedOutput(dir, Owns)
		outs.Go = goOut
	}

	for _, target := range cfg.Targets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch target {
		case message.TargetBackend:
			if err := GoBundle(goOut, cfg, set); err != nil {
				return nil, err
			}
			if err := GoCode(goOut, cfg, set); err != nil {
				return nil, err
			}
		case message.TargetWeb, message.TargetDesktop:
			if err := Vue(out, cfg, set, target); err != nil {
				return nil, err
			}
			if err := Plurals(out, cfg, target); err != nil {
				return nil, err
			}
			if err := Keys(out, cfg, set, target); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("generate: unknown target %q", target)
		}
	}

	if err := addManifest(out, cfg, set); err != nil {
		return nil, err
	}
	return outs, nil
}

func addManifest(out *Output, cfg *config.Config, set *message.Set) error {
	man := Manifest{
		SchemaVersion: 1,
		SourceHash:    sourceHash(cfg, set),
		DefaultLocale: cfg.DefaultLocale,
		Locales:       cfg.Locales,
		Targets:       map[string]TargetManifest{},
		Files:         map[string]string{},
	}

	for _, target := range cfg.Targets {
		var namespaces []string
		count := 0

		for _, key := range set.Keys() {
			m, ok := set.Get(cfg.DefaultLocale, key)
			if !ok || !cfg.Wants(m, target) {
				continue
			}
			count++
			if !slices.Contains(namespaces, m.Namespace) {
				namespaces = append(namespaces, m.Namespace)
			}
		}
		sort.Strings(namespaces)

		man.Targets[string(target)] = TargetManifest{Namespaces: namespaces, Messages: count}
	}

	// Content hashes let a consumer tell whether a bundle it cached is current.
	for _, f := range out.Files() {
		sum := sha256.Sum256(f.Bytes)
		man.Files[f.Path] = hex.EncodeToString(sum[:])
	}

	return out.AddJSON("manifest.json", man)
}

// sourceHash fingerprints the inputs, so `check` can tell a stale build from a
// config change without re-reading every source file.
func sourceHash(cfg *config.Config, set *message.Set) string {
	h := sha256.New()

	fmt.Fprintf(h, "v1\n%s\n", cfg.DefaultLocale)
	for _, l := range cfg.Locales {
		fmt.Fprintf(h, "locale:%s\n", l)
	}
	for _, t := range cfg.Targets {
		fmt.Fprintf(h, "target:%s\n", t)
	}

	for _, key := range set.Keys() {
		for _, locale := range cfg.Locales {
			m, ok := set.Get(locale, key)
			if !ok {
				continue
			}
			fmt.Fprintf(h, "%s\x00%s\x00%s\x00", locale, key, m.Kind)
			for _, text := range m.Text() {
				fmt.Fprintf(h, "%s\x00", text)
			}
			for _, p := range message.SortedPlaceholders(m.Placeholders) {
				fmt.Fprintf(h, "%s:%s\x00", p.Name, p.Type)
			}
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
