// cmd/loc/load.go

package main

import (
	"context"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/validate"
)

// load runs config, parse and validate, returning the report unprinted for `check --json`.
func load(ctx context.Context, a *app) (*config.Config, *message.Set, *validate.Report, error) {
	cfg, err := config.Load(ctx, a.opts.configPath)
	if err != nil {
		return nil, nil, nil, err
	}

	res, err := parse.Roots(ctx, cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		return nil, nil, nil, err
	}

	// Non-nil whenever validation ran, including when it failed.
	report := validate.Run(cfg, res)
	if report.HasErrors() || (a.opts.strict && len(report.Warnings()) > 0) {
		return cfg, res.Set, report, errFailed
	}
	return cfg, res.Set, report, nil
}

// loadAndReport is load with the diagnostics printed.
func loadAndReport(ctx context.Context, a *app) (*config.Config, *message.Set, error) {
	cfg, set, report, err := load(ctx, a)
	if report != nil {
		a.printReport(report)
	}
	return cfg, set, err
}
