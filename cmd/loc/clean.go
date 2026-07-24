// cmd/loc/clean.go

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/lock"
	"github.com/smegg99/s99locale/internal/message"
)

func newCleanCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove generated artefacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdClean(cmd.Context(), a)
		},
	}
}

func cmdClean(ctx context.Context, a *app) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	cfg, err := config.Load(a.opts.configPath)
	if err != nil {
		return err
	}

	dir := cfg.AbsOutput()
	if err := removable(cfg, dir); err != nil {
		return err
	}

	held, err := lock.Acquire(dir, true)
	if err != nil {
		return err
	}
	defer held.Release()

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		a.says("loc: nothing to clean\n")
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clean %s: %w", dir, err)
	}
	a.says("loc: removed %s\n", dir)

	// A Go target placed in the package that embeds it shares that directory
	// with hand-written source, so only what loc owns there goes.
	if cfg.Redirected(message.TargetBackend) {
		shared := cfg.AbsTargetOutput(message.TargetBackend)
		removed, err := removeOwned(shared)
		if err != nil {
			return err
		}
		for _, name := range removed {
			a.says("loc: removed %s\n", filepath.Join(shared, name))
		}
	}
	return nil
}

// removeOwned deletes the files loc generates inside a shared package, and
// nothing else in it.
func removeOwned(dir string) ([]string, error) {
	var removed []string

	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && p == dir {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		mine, err := generate.Owns(dir, filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		if !mine {
			return nil
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		removed = append(removed, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return removed, fmt.Errorf("clean %s: %w", dir, err)
	}
	sort.Strings(removed)
	return removed, nil
}

// removable decides whether RemoveAll may be pointed at the output directory.
//
// `output` is a config string and clean runs unattended, so a typo has to be
// caught here rather than discovered afterwards: `output: ".."` would take the
// canonical source and everything beside it. Nothing outside the config's own
// directory is ever removed, and nothing that holds a source root.
func removable(cfg *config.Config, dir string) error {
	base, err := filepath.Abs(cfg.Dir)
	if err != nil {
		return fmt.Errorf("clean: %w", err)
	}
	target, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("clean: %w", err)
	}

	if target == base {
		return fmt.Errorf("refusing to clean %s: output is the config directory", dir)
	}
	if !under(base, target) {
		return fmt.Errorf("refusing to clean %s: output is outside %s", dir, cfg.Dir)
	}

	for _, root := range append(cfg.AbsRoots(), cfg.AbsScan()...) {
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if under(target, abs) {
			return fmt.Errorf("refusing to clean %s: it holds the source root %s", dir, root)
		}
	}
	return nil
}

// under reports whether child is at or beneath parent.
func under(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
