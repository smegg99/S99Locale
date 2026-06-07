// cmd/loc/import.go

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/legacy"
)

// cmdImport reads existing locale trees into canonical YAML.
//
// It never overwrites without --force: an import over hand-edited canonical
// files would silently drop every description someone had written.
func cmdImport(ctx context.Context, configPath string, src legacy.Sources, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if src.Go == "" && src.Vue == "" {
		return fmt.Errorf("nothing to import: pass --from-go, --from-vue, or both")
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	res, err := legacy.Import(src, cfg.Locales)
	if err != nil {
		return err
	}

	// The first root is where a library's strings live; an import belongs in
	// the app's own root, which is the last one.
	roots := cfg.AbsRoots()
	dest := roots[len(roots)-1]

	written, skipped, err := writeCanonical(dest, res.Files, force)
	if err != nil {
		return err
	}

	says("loc: %d message(s) into %d file(s), %d written, %d already present\n",
		res.Count, len(res.Files), written, skipped)

	if skipped > 0 && !force {
		fmt.Println("     existing files were left alone; pass --force to replace them")
	}
	printNotes(res.Notes)
	return nil
}

func writeCanonical(dest string, files map[string][]byte, force bool) (written, skipped int, err error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, rel := range paths {
		full := filepath.Join(dest, filepath.FromSlash(rel))

		if _, statErr := os.Stat(full); statErr == nil && !force {
			skipped++
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return written, skipped, fmt.Errorf("import: mkdir: %w", err)
		}
		if err := os.WriteFile(full, files[rel], 0o644); err != nil {
			return written, skipped, fmt.Errorf("import: write %s: %w", full, err)
		}
		written++
	}
	return written, skipped, nil
}

// printNotes reports what the import guessed rather than knew.
func printNotes(notes []legacy.Note) {
	if len(notes) == 0 {
		return
	}

	fmt.Fprintf(os.Stderr, "\n%d thing(s) need a human:\n", len(notes))
	for _, n := range notes {
		fmt.Fprintf(os.Stderr, "  %-32s %-3s %s\n", n.Key, n.Locale, n.Detail)
	}
	fmt.Fprintln(os.Stderr, "\nDescriptions were not recoverable from any bundle and are worth adding.")
}
