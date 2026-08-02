// cmd/loc/clean.go

package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/lock"
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
