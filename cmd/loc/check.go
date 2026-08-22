// cmd/loc/check.go

package main

import (
	"context"
	"encoding/json"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/lock"
	"github.com/smegg99/s99locale/internal/validate"
)

// CheckResult is the `check --json` document.
type CheckResult struct {
	Current     bool                  `json:"current"`
	Files       int                   `json:"files"`
	Stale       []string              `json:"stale"`
	Modified    []string              `json:"modified"`
	Errors      int                   `json:"errors"`
	Warnings    int                   `json:"warnings"`
	Diagnostics []validate.Diagnostic `json:"diagnostics"`
}

func newCheckCmd(a *app) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:  "check",
		Args: a.noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdCheck(cmd.Context(), a, asJSON)
		},
	}

	cmd.Flags().BoolVar(&a.opts.strict, "strict", false, "")
	cmd.Flags().BoolVar(&asJSON, "json", false, "")

	a.describe(func(l *i18n.Localizer) {
		cmd.Short = catalog.CliCheckShort(l)
		cmd.Long = catalog.CliCheckLong(l)
		cmd.Flags().Lookup("strict").Usage = catalog.CliFlagStrictCheck(l)
		cmd.Flags().Lookup("json").Usage = catalog.CliFlagJSON(l)
	})

	return cmd
}

// cmdCheck never writes; CI runs it.
func cmdCheck(ctx context.Context, a *app, asJSON bool) error {
	cfg, set, report, err := load(ctx, a)
	if err != nil {
		// A config that fails to load has no report, so --json gets the error on stderr and exit 2.
		if report == nil || !asJSON {
			if report != nil {
				a.printReport(report)
			}
			return err
		}
		return a.checkJSON(report, nil, nil, 0, err)
	}

	// Shared lock over every tree a build writes or clears, retired ones included: check only reads.
	held, err := lock.AcquireAll(ctx, generate.Dirs(cfg), false, a.waitingForLock)
	if err != nil {
		return err
	}
	defer held.Release()

	out, err := generate.Build(ctx, cfg, set)
	if err != nil {
		return err
	}

	stale, modified, err := out.Diff()
	if err != nil {
		return err
	}

	if asJSON {
		return a.checkJSON(report, stale, modified, len(out.Files()), nil)
	}
	a.printReport(report)

	// Two classes because the remedy differs: `loc build` fixes a stale file but refuses one
	// edited since or never written by loc.
	if len(modified) > 0 {
		a.warns("loc cannot prove it wrote these, so build will not touch them; remove them by hand:\n")
		for _, p := range modified {
			a.warns("  %s\n", p)
		}
	}
	if len(stale) > 0 {
		a.warns("generated output is stale; run `loc build`:\n")
		for _, p := range stale {
			a.warns("  %s\n", p)
		}
	}
	if len(stale)+len(modified) > 0 {
		return errFailed
	}

	a.says("loc: %d file(s) current\n", len(out.Files()))
	return nil
}

// checkJSON writes the result to the data stream and keeps the human run's exit code.
func (a *app) checkJSON(report *validate.Report, stale, modified []string, files int, loadErr error) error {
	res := CheckResult{
		Current:     loadErr == nil && len(stale) == 0 && len(modified) == 0,
		Files:       files,
		Stale:       stale,
		Modified:    modified,
		Errors:      len(report.Errors()),
		Warnings:    len(report.Warnings()),
		Diagnostics: report.Diagnostics,
	}
	if res.Stale == nil {
		res.Stale = []string{}
	}
	if res.Modified == nil {
		res.Modified = []string{}
	}
	if res.Diagnostics == nil {
		res.Diagnostics = []validate.Diagnostic{}
	}

	enc := json.NewEncoder(a.data)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		return err
	}
	if !res.Current {
		return errFailed
	}
	return nil
}
