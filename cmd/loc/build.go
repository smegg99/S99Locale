// cmd/loc/build.go

package main

import (
	"context"
	"strconv"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/smegg99/s99term"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/catalog"

	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/lock"
)

func newBuildCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "build",
		Args: a.noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdBuild(cmd.Context(), a)
		},
	}
	cmd.Flags().BoolVar(&a.opts.strict, "strict", false, "")

	a.describe(func(l *i18n.Localizer) {
		cmd.Short = catalog.CliBuildShort(l)
		cmd.Flags().Lookup("strict").Usage = catalog.CliFlagStrictBuild(l)
	})
	return cmd
}

func cmdBuild(ctx context.Context, a *app) error {
	cfg, set, err := loadAndReport(ctx, a)
	if err != nil {
		return err
	}

	// Exclusive over every directory Dirs names, retired trees included.
	held, err := lock.AcquireAll(ctx, generate.Dirs(cfg), true, a.waitingForLock)
	if err != nil {
		return err
	}
	defer held.Release()

	step := a.begin(catalog.ReportBuildWorking(a.say()))

	out, err := generate.Build(ctx, cfg, set)
	if err != nil {
		a.end(step, s99term.StatusFailed)
		return err
	}

	written, removed, err := out.Write()
	if err != nil {
		a.end(step, s99term.StatusFailed)
		// Trees written before the failure stay written, so their removals are reported with the error.
		for _, name := range removed {
			a.warns(s99term.StatusSkipped, catalog.ReportBuildRemoved(a.say(), catalog.ReportBuildRemovedParams{
				Path: name}))
		}
		return err
	}
	a.end(step, s99term.StatusOK, s99term.F("files", strconv.Itoa(len(out.Files()))))

	a.says(s99term.StatusOK, catalog.ReportBuildGenerated(a.say(), catalog.ReportBuildGeneratedParams{
		Files: len(out.Files()), Changed: written}))
	for _, name := range removed {
		a.says(s99term.StatusSkipped, catalog.ReportBuildRemoved(a.say(), catalog.ReportBuildRemovedParams{
			Path: name}))
	}
	return nil
}
