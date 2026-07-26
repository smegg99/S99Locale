// cmd/loc/build.go

package main

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/lock"
)

func newBuildCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Validate and generate every configured target",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdBuild(cmd.Context(), a)
		},
	}
	cmd.Flags().BoolVar(&a.opts.strict, "strict", false, "warnings fail the build")
	return cmd
}

func cmdBuild(ctx context.Context, a *app) error {
	cfg, set, err := loadAndReport(ctx, a)
	if err != nil {
		return err
	}

	// Exclusive: this writes, and orphan detection reads a tree another run
	// could be writing to.
	held, err := lock.Acquire(cfg.AbsOutput(), true)
	if err != nil {
		return err
	}
	defer held.Release()

	out, err := generate.Build(ctx, cfg, set)
	if err != nil {
		return err
	}

	written, removed, err := out.Write()
	if err != nil {
		// Trees written before the failure stay written, so their removals are reported with the error.
		for _, name := range removed {
			a.warns("loc: removed %s\n", name)
		}
		return err
	}

	a.says("loc: %d file(s) generated, %d changed\n", len(out.Files()), written)
	for _, name := range removed {
		a.says("loc: removed %s\n", name)
	}
	return nil
}
