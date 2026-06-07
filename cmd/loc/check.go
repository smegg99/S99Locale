// cmd/loc/check.go

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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

// cmdCheck never writes; CI runs it.
func cmdCheck(ctx context.Context, path string, strict, asJSON bool) error {
	cfg, set, report, err := load(ctx, path, strict)
	if err != nil {
		// A config that would not load has no report to render, so JSON callers
		// get the error on stderr and exit 2 like everyone else.
		if report == nil || !asJSON {
			if report != nil {
				printReport(report)
			}
			return err
		}
		return checkJSON(report, nil, 0, err)
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
		return checkJSON(report, stale, len(out.Files()), nil)
	}
	printReport(report)

	if len(stale) > 0 {
		fmt.Fprintf(os.Stderr, "generated output is stale; run `loc build`:\n")
		for _, p := range stale {
			fmt.Fprintf(os.Stderr, "  %s\n", p)
		}
		return errFailed
	}

	says("loc: %d file(s) current\n", len(out.Files()))
	return nil
}

// checkJSON writes the result to stdout and still fails the process, so a
// machine-readable run has the same exit code as a human one.
func checkJSON(report *validate.Report, stale []string, files int, loadErr error) error {
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

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(res); err != nil {
		return err
	}
	if !res.Current {
		return errFailed
	}
	return nil
}
