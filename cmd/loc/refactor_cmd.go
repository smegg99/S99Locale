// cmd/loc/refactor_cmd.go

package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/refactor"
	"github.com/smegg99/s99locale/internal/scan"
)

// cmdRefactorShared prints a patch. It never writes source.
//
// A scan cannot see dynamic references, so a patch generated from one can be
// incomplete and still look complete.
func cmdRefactorShared(ctx context.Context, configPath string, from []string, to string) error {
	cfg, set, err := loadAndReport(ctx, configPath, false)
	if err != nil {
		return err
	}

	refs, err := references(cfg, set)
	if err != nil {
		return err
	}

	plan, err := refactor.PlanRename(cfg, set, refs, from, to)
	if err != nil {
		var refusal *refactor.Refusal
		if errors.As(err, &refusal) {
			fmt.Fprintln(os.Stderr, err)
			return errFailed
		}
		return err
	}

	fmt.Printf("Rename %d key(s) to %s\n\n", len(plan.From), plan.To)

	if patch := plan.Patch(); patch != "" {
		fmt.Println(patch)
	}

	if len(plan.Notes) > 0 {
		fmt.Println("Then, by hand:")
		for _, note := range plan.Notes {
			fmt.Printf("  - %s\n", note)
		}
		fmt.Println()
	}

	fmt.Println("Nothing was written. Apply the patch, make the edits above, then run `loc check`.")
	return nil
}

// references scans for key usages, mapping generated Go names back to keys.
func references(cfg *config.Config, set *message.Set) (*scan.Result, error) {
	if len(cfg.Scan) == 0 {
		return &scan.Result{Refs: map[string][]scan.Reference{}}, nil
	}

	extra, err := scan.Compile(cfg.KeyPatterns)
	if err != nil {
		return nil, err
	}

	return scan.Run(scan.Options{
		Roots:     cfg.AbsScan(),
		Generated: cfg.AbsOutput(),
		GoNames:   generate.GoNames(cfg, set),
		Extra:     extra,
	})
}
