// cmd/loc/watch.go

package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/config"
)

// debounce is how long to wait after the last event.
//
// An editor writes a file as several operations - truncate, write, rename - and
// rebuilding on each produces three builds and two of them read a half-written
// file.
const debounce = 150 * time.Millisecond

func newWatchCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Rebuild on every change until interrupted",
		Long: "Survives a validation error and keeps watching: a half-typed key is\n" +
			"the normal state of a file being edited.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdWatch(cmd.Context(), a)
		},
	}
	cmd.Flags().BoolVar(&a.opts.strict, "strict", false, "warnings fail the rebuild")
	return cmd
}

// cmdWatch rebuilds on change until interrupted.
//
// A validation error prints and the watcher keeps going: a half-typed key is
// the normal state of a file being edited.
func cmdWatch(ctx context.Context, a *app) error {
	cfg, err := config.Load(a.opts.configPath)
	if err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	defer watcher.Close()

	// The config itself, so changing a locale or a target takes effect.
	if err := watcher.Add(filepath.Dir(a.opts.configPath)); err != nil {
		return fmt.Errorf("watch: %s: %w", a.opts.configPath, err)
	}
	for _, root := range cfg.AbsRoots() {
		if err := watchTree(watcher, root); err != nil {
			return err
		}
	}

	a.says("loc: watching %d root(s); ctrl-c to stop\n", len(cfg.AbsRoots()))
	rebuild(ctx, a)

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	for {
		select {
		case <-ctx.Done():
			a.warns("\nloc: stopped\n")
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !relevant(event) {
				continue
			}
			// Watch new directories, or files created in them are missed.
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					_ = watchTree(watcher, event.Name)
				}
			}
			timer.Reset(debounce)

		case <-timer.C:
			rebuild(ctx, a)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			a.warns("loc: watch: %v\n", err)
		}
	}
}

// relevant filters the events worth rebuilding for. Chmod fires on reads on some systems.
func relevant(event fsnotify.Event) bool {
	if event.Has(fsnotify.Chmod) && !event.Has(fsnotify.Write) {
		return false
	}
	switch filepath.Ext(event.Name) {
	case ".yaml", ".yml", ".cue":
		return true
	}
	return false
}

func watchTree(watcher *fsnotify.Watcher, root string) error {
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			return nil
		}
		return watcher.Add(path)
	})
	if err != nil {
		return fmt.Errorf("watch: %s: %w", root, err)
	}
	return nil
}

// rebuild runs one build and reports, swallowing failure so the loop survives.
func rebuild(ctx context.Context, a *app) {
	stamp := time.Now().Format("15:04:05")

	if err := cmdBuild(ctx, a); err != nil {
		// cmdBuild printed the diagnostics; this only marks the loop alive.
		a.warns("%s  build failed; still watching\n", stamp)
		return
	}
	a.says("%s  ok\n", stamp)
}
