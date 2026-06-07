// cmd/loc/clean.go

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/lock"
)

func cmdClean(path string) error {
	cfg, err := config.Load(path)
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
		says("loc: nothing to clean\n")
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clean %s: %w", dir, err)
	}

	says("loc: removed %s\n", dir)
	return nil
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
