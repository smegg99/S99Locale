// cmd/loc/clean.go

package main

import (
	"context"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/lock"
)

func newCleanCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "clean",
		Args: a.noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdClean(cmd.Context(), a)
		},
	}

	a.describe(func(l *i18n.Localizer) { cmd.Short = catalog.CliCleanShort(l) })
	return cmd
}

func cmdClean(ctx context.Context, a *app) error {
	cfg, err := config.Load(ctx, a.opts.configPath)
	if err != nil {
		return err
	}

	// Exclusive over every directory a build writes or clears, retired ones included.
	held, err := lock.AcquireAll(ctx, generate.Dirs(cfg), true, a.waitingForLock)
	if err != nil {
		return err
	}
	defer held.Release()

	removed, err := generate.Clean(cfg)
	if err != nil {
		// Trees cleared before the failure stay cleared, so their removals are reported with the error.
		for _, name := range removed {
			a.warns("loc: removed %s\n", name)
		}
		return err
	}

	if len(removed) == 0 {
		a.says("loc: nothing to clean\n")
		return nil
	}
	for _, name := range removed {
		a.says("loc: removed %s\n", name)
	}
	return nil
}
