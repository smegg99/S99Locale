// cmd/loc/watch.go

package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/smegg99/s99term"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/config"
)

// debounce covers the several writes an editor makes for one save.
const debounce = 150 * time.Millisecond

func newWatchCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "watch",
		Args: a.noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdWatch(cmd.Context(), a)
		},
	}
	cmd.Flags().BoolVar(&a.opts.strict, "strict", false, "")

	a.describe(func(l *i18n.Localizer) {
		cmd.Short = catalog.CliWatchShort(l)
		cmd.Long = catalog.CliWatchLong(l)
		cmd.Flags().Lookup("strict").Usage = catalog.CliFlagStrictWatch(l)
	})
	return cmd
}

// cmdWatch rebuilds on change until interrupted; a validation error prints and watching goes on.
func cmdWatch(ctx context.Context, a *app) error {
	cfg, err := config.Load(ctx, a.opts.configPath)
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

	a.says(s99term.StatusRunning, catalog.ReportWatchWatching(a.say(), catalog.ReportWatchWatchingParams{
		Roots: len(cfg.AbsRoots())}))
	rebuild(ctx, a)

	timer := time.NewTimer(0)
	if !timer.Stop() {
		<-timer.C
	}

	for {
		select {
		case <-ctx.Done():
			a.warns(s99term.StatusSkipped, catalog.ReportWatchStopped(a.say()))
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			info, err := os.Stat(event.Name)
			watch, again := react(event, err == nil && info.IsDir())
			if watch {
				_ = watchTree(watcher, event.Name)
			}
			if again {
				timer.Reset(debounce)
			}

		case <-timer.C:
			rebuild(ctx, a)

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			a.warns(s99term.StatusWarning, catalog.ReportWatchError(a.say(), catalog.ReportWatchErrorParams{
				Error: err.Error()}))
		}
	}
}

// react watches a created directory and rebuilds in case it arrived full; a source file rebuilds.
func react(event fsnotify.Event, dir bool) (watch, rebuild bool) {
	if dir && event.Has(fsnotify.Create) {
		return true, true
	}
	return false, relevant(event)
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
	start := time.Now()

	err := cmdBuild(ctx, a)
	if err == nil {
		a.says(s99term.StatusOK, catalog.ReportWatchOk(a.say()), s99term.Dur("took", time.Since(start)))
		return
	}
	// ctrl-c ended the build, usually a lock wait; a failure line would blame the sources.
	if ctx.Err() != nil {
		return
	}
	// cmdBuild prints diagnostics for errFailed only; any other error is printed here.
	if !errors.Is(err, errFailed) {
		a.warns(s99term.StatusFailed, err.Error())
	}
	a.warns(s99term.StatusFailed, catalog.ReportWatchFailed(a.say()), s99term.Dur("took", time.Since(start)))
}
