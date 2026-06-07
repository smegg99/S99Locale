// cmd/loc/commands.go

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/lock"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/validate"
)

// load runs the whole front half: config, parse, validate.
//
// The report comes back rather than being printed here, because `check --json`
// has to render the same diagnostics as JSON instead. It is non-nil whenever
// validation ran, including when validation is what failed.
func load(ctx context.Context, path string, strict bool) (*config.Config, *message.Set, *validate.Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}

	cfg, err := config.Load(path)
	if err != nil {
		return nil, nil, nil, err
	}

	res, err := parse.Roots(ctx, cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		return nil, nil, nil, err
	}

	report := validate.Run(cfg, res, strict)
	if report.HasErrors() || (strict && len(report.Warnings()) > 0) {
		return cfg, res.Set, report, errFailed
	}
	return cfg, res.Set, report, nil
}

// loadAndReport is load with the diagnostics printed.
func loadAndReport(ctx context.Context, path string, strict bool) (*config.Config, *message.Set, error) {
	cfg, set, report, err := load(ctx, path, strict)
	if report != nil {
		printReport(report)
	}
	return cfg, set, err
}

// quiet suppresses the lines that say a run went fine. It must never reach
// printReport or the stale list: a quiet run that fails still has to say why.
var quiet bool

// says prints a success line unless --quiet.
func says(format string, args ...any) {
	if quiet {
		return
	}
	fmt.Printf(format, args...)
}

func printReport(r *validate.Report) {
	for _, d := range r.Diagnostics {
		fmt.Fprintln(os.Stderr, d.String())
	}

	errs, warns := len(r.Errors()), len(r.Warnings())
	if errs+warns > 0 {
		fmt.Fprintf(os.Stderr, "%d error(s), %d warning(s)\n", errs, warns)
	}
}

func cmdBuild(ctx context.Context, path string, strict bool) error {
	cfg, set, err := loadAndReport(ctx, path, strict)
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
		return err
	}

	total := len(out.Files())
	if removed > 0 {
		// Named, because a silent deletion under a generated tree is the kind
		// of thing someone later has to bisect for.
		says("loc: %d file(s) generated, %d changed, %d removed\n", total, written, removed)
		return nil
	}
	says("loc: %d file(s) generated, %d changed\n", total, written)
	return nil
}
