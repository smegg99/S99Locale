// cmd/loc/check.go

package main

import (
	"context"
	"encoding/json"

	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/lock"
	"github.com/smegg99/s99locale/internal/validate"
)

// CheckResult is the `check --json` document.
type CheckResult struct {
	Current     bool                  `json:"current"`
	Files       int                   `json:"files"`
	Stale       []string              `json:"stale"`
	Errors      int                   `json:"errors"`
	Warnings    int                   `json:"warnings"`
	Diagnostics []validate.Diagnostic `json:"diagnostics"`
}

func newCheckCmd(a *app) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate and confirm the generated output is current",
		Long: "Never writes, which is what CI runs. Exit 1 means the locales are\n" +
			"wrong or the output is stale; exit 2 means loc could not run at all.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdCheck(cmd.Context(), a, asJSON)
		},
	}

	cmd.Flags().BoolVar(&a.opts.strict, "strict", false, "warnings fail the check")
	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")

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
		return a.checkJSON(report, nil, 0, err)
	}

	// Shared: check only reads, so two CI jobs checking at once need not queue.
	held, err := lock.Acquire(cfg.AbsOutput(), false)
	if err != nil {
		return err
	}
	defer held.Release()

	out, err := generate.Build(ctx, cfg, set)
	if err != nil {
		return err
	}

	stale, err := out.Diff()
	if err != nil {
		return err
	}

	if asJSON {
		return a.checkJSON(report, stale, len(out.Files()), nil)
	}
	a.printReport(report)

	if len(stale) > 0 {
		a.warns("generated output is stale; run `loc build`:\n")
		for _, p := range stale {
			a.warns("  %s\n", p)
		}
		return errFailed
	}

	a.says("loc: %d file(s) current\n", len(out.Files()))
	return nil
}

// checkJSON writes the result to the data stream and keeps the human run's exit code.
func (a *app) checkJSON(report *validate.Report, stale []string, files int, loadErr error) error {
	res := CheckResult{
		Current:     loadErr == nil && len(stale) == 0,
		Files:       files,
		Stale:       stale,
		Errors:      len(report.Errors()),
		Warnings:    len(report.Warnings()),
		Diagnostics: report.Diagnostics,
	}
	if res.Stale == nil {
		res.Stale = []string{}
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
