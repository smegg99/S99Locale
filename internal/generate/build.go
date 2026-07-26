// internal/generate/build.go

package generate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
)

// ManifestName is the build record in every target; "manifest.json" would clash with PWAs.
const ManifestName = "loc-manifest.json"

// ManifestSchemaVersion is the record's shape; a record with any other value is an error.
const ManifestSchemaVersion = 2

// Manifest describes one target's build. No timestamps, so identical builds match.
type Manifest struct {
	SchemaVersion int      `json:"schemaVersion"`
	SourceHash    string   `json:"sourceHash"`
	Target        string   `json:"target"`
	DefaultLocale string   `json:"defaultLocale"`
	Locales       []string `json:"locales"`
	Namespaces    []string `json:"namespaces"`
	Messages      int      `json:"messages"`
	// Files maps each path, relative to this directory, to the sha256 loc wrote.
	Files map[string]string `json:"files"`
}

// Build generates every configured target into its own tree, without writing.
func Build(ctx context.Context, cfg *config.Config, set *message.Set) (*Outputs, error) {
	outs := newOutputs()
	for _, target := range cfg.Targets {
		outs.add(NewOutput(cfg.AbsTargetOutput(target), cfg.TargetLabel(target), target))
	}
	live, err := liveDirs(cfg)
	if err != nil {
		return nil, err
	}
	old, err := retired(cfg, live)
	if err != nil {
		return nil, err
	}
	outs.retired = old

	for _, target := range cfg.Targets {
		// Cancellation is checked between targets; each one is a whole bundle.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out := outs.For(target)

		switch target {
		case message.TargetBackend:
			if err := GoBundle(out, cfg, set); err != nil {
				return nil, err
			}
			if err := GoCode(out, cfg, set); err != nil {
				return nil, err
			}
		case message.TargetWeb:
			if err := Vue(out, cfg, set); err != nil {
				return nil, err
			}
			if err := Plurals(out, cfg); err != nil {
				return nil, err
			}
			if err := Keys(out, cfg); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("generate: unknown target %q", target)
		}

		if err := addManifest(out, cfg, set); err != nil {
			return nil, err
		}
	}
	return outs, nil
}

// liveDirs stats each existing target directory and refuses two that are one directory.
func liveDirs(cfg *config.Config) ([]os.FileInfo, error) {
	// config compares paths as text; a symlink can still make two of them one directory.
	var infos []os.FileInfo
	var owners []message.Target
	for _, t := range cfg.Targets {
		dir := cfg.AbsTargetOutput(t)
		info, err := os.Stat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("generate: stat %s: %w", dir, err)
		}
		if i := sameDir(infos, info); i >= 0 {
			return nil, fmt.Errorf("generate: targets %q and %q both write into %s: %s is the same directory",
				owners[i], t, cfg.TargetLabel(t), cfg.TargetLabel(owners[i]))
		}
		infos = append(infos, info)
		owners = append(owners, t)
	}
	return infos, nil
}

// retired is an empty tree at each default target location this config does not write.
func retired(cfg *config.Config, live []os.FileInfo) ([]*Output, error) {
	// A dropped or redirected target leaves its last build at <output>/<target>.
	// A redirected directory dropped from the config is not found here; remove it by hand.
	var out []*Output
	var infos []os.FileInfo
	var links []bool
	for _, t := range []message.Target{message.TargetBackend, message.TargetWeb} {
		if slices.Contains(cfg.Targets, t) && !cfg.Redirected(t) {
			continue
		}
		dir := filepath.Join(cfg.AbsOutput(), string(t))
		info, err := os.Stat(dir)
		if errors.Is(err, fs.ErrNotExist) {
			// A path with no directory has no record, so there is nothing to clear.
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("generate: stat %s: %w", dir, err)
		}
		if sameDir(live, info) >= 0 {
			// A symlink to a live directory: clearing it would remove this build's files.
			continue
		}
		own, err := os.Lstat(dir)
		if err != nil {
			return nil, fmt.Errorf("generate: stat %s: %w", dir, err)
		}
		link := own.Mode()&os.ModeSymlink != 0

		label, err := filepath.Rel(cfg.Dir, dir)
		if err != nil {
			label = dir
		}
		tree := NewOutput(dir, filepath.ToSlash(label), t)

		// Two old paths to one directory are cleared once, through the symlink, which survives.
		i := sameDir(infos, info)
		switch {
		case i < 0:
			out = append(out, tree)
			infos = append(infos, info)
			links = append(links, link)
		case link && !links[i]:
			out[i], links[i] = tree, true
		}
	}
	return out, nil
}

// sameDir is the index of the entry in infos that is the directory info describes, or -1.
func sameDir(infos []os.FileInfo, info os.FileInfo) int {
	return slices.IndexFunc(infos, func(one os.FileInfo) bool { return os.SameFile(one, info) })
}

// addManifest records this target's build so the next build's sweep can check against it.
func addManifest(out *Output, cfg *config.Config, set *message.Set) error {
	namespaces := []string{}
	count := 0

	for _, key := range set.Keys() {
		m, ok := set.Get(cfg.DefaultLocale, key)
		if !ok || !cfg.Wants(m, out.Target) {
			continue
		}
		count++
		if !slices.Contains(namespaces, m.Namespace) {
			namespaces = append(namespaces, m.Namespace)
		}
	}
	sort.Strings(namespaces)

	man := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		SourceHash:    sourceHash(cfg, set),
		Target:        string(out.Target),
		DefaultLocale: cfg.DefaultLocale,
		Locales:       cfg.Locales,
		Namespaces:    namespaces,
		Messages:      count,
		Files:         map[string]string{},
	}

	// Paths are relative to this directory, so the record does not depend on -c.
	for _, f := range out.Files() {
		man.Files[f.Path] = hashOf(f.Bytes)
	}

	return out.AddJSON(ManifestName, man)
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
