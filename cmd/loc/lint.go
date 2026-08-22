// cmd/loc/lint.go

package main

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/scan"
	"github.com/smegg99/s99locale/internal/shared"
)

func newLintCmd(a *app) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:  "lint",
		Args: a.noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdLintShared(cmd.Context(), a, false, asJSON)
		},
	}

	cmd.PersistentFlags().BoolVar(&a.opts.strict, "strict", false, "")
	cmd.PersistentFlags().BoolVar(&asJSON, "json", false, "")

	shared := &cobra.Command{
		Use:  "shared",
		Args: a.noArgs,
	}
	var verbose bool
	shared.Flags().BoolVar(&verbose, "verbose", false, "")
	shared.RunE = func(cmd *cobra.Command, _ []string) error {
		return cmdLintShared(cmd.Context(), a, verbose, asJSON)
	}

	keys := &cobra.Command{
		Use:  "keys",
		Args: a.noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdLintKeys(cmd.Context(), a, asJSON)
		},
	}
	duplicates := &cobra.Command{
		Use:  "duplicates",
		Args: a.noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdLintDuplicates(cmd.Context(), a, asJSON)
		},
	}

	cmd.AddCommand(shared, keys, duplicates)

	a.describe(func(l *i18n.Localizer) {
		cmd.Short = catalog.CliLintShort(l)
		cmd.Long = catalog.CliLintLong(l)
		// --strict and --json are persistent here, so the lookup names PersistentFlags.
		cmd.PersistentFlags().Lookup("strict").Usage = catalog.CliFlagStrictLint(l)
		cmd.PersistentFlags().Lookup("json").Usage = catalog.CliFlagJSON(l)
		shared.Short = catalog.CliLintSharedShort(l)
		shared.Flags().Lookup("verbose").Usage = catalog.CliFlagVerbose(l)
		keys.Short = catalog.CliLintKeysShort(l)
		duplicates.Short = catalog.CliLintDuplicatesShort(l)
	})
	return cmd
}

// cmdLintShared reports keys that may be one message.
//
// Only high confidence is shown by default; medium and low are right often
// enough to keep but wrong often enough to hide behind --verbose.
func cmdLintShared(ctx context.Context, a *app, verbose, asJSON bool) error {
	cfg, set, err := loadAndReport(ctx, a)
	if err != nil {
		return err
	}

	all := shared.Detect(cfg, set)

	var shown []shared.Candidate
	for _, c := range all {
		if verbose || c.Confidence == shared.High {
			shown = append(shown, c)
		}
	}

	if asJSON {
		enc := json.NewEncoder(a.data)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"candidates": shown})
	}

	if len(shown) == 0 {
		a.says("loc: no shared-message candidates\n")
		if hidden := len(all) - len(shown); hidden > 0 {
			a.says("     %d lower-confidence finding(s); see `loc lint shared --verbose`\n", hidden)
		}
		return nil
	}

	for _, c := range shown {
		printCandidate(a, c)
	}

	a.warns("%d candidate(s)\n", len(shown))
	if hidden := len(all) - len(shown); hidden > 0 {
		a.warns("%d lower-confidence finding(s) hidden; see `loc lint shared --verbose`\n", hidden)
	}
	return nil
}

func printCandidate(a *app, c shared.Candidate) {
	a.warns("\nI18N021 possible shared message\n\n")

	a.warns("  Keys:\n")
	for _, key := range c.Keys {
		a.warns("    %s\n", key)
	}

	a.warns("\n  Translations:\n")
	for _, locale := range sortedMapKeys(c.Translations) {
		// A plural's forms are joined by a NUL; show the separator instead.
		text := strings.ReplaceAll(c.Translations[locale], "\x00", " | ")
		a.warns("    %-4s %s\n", locale+":", text)
	}

	if c.Suggested != "" {
		a.warns("\n  Suggested key:\n    %s\n", c.Suggested)
	}
	a.warns("\n  Confidence:\n    %s\n", c.Confidence)
	if c.Reason != "" {
		a.warns("    %s\n", reasonText(a.say(), c))
	}
}

// reasonText is a downgrade's reason in words; --json keeps the code.
func reasonText(l *i18n.Localizer, c shared.Candidate) string {
	switch c.Reason {
	case shared.ReasonOneLocale:
		return catalog.ReportLintReasonOneLocale(l)
	case shared.ReasonDescriptions:
		return catalog.ReportLintReasonDescriptions(l)
	case shared.ReasonTargets:
		return catalog.ReportLintReasonTargets(l)
	case shared.ReasonContract:
		return catalog.ReportLintReasonContract(l)
	case shared.ReasonDefaultOnly:
		return catalog.ReportLintReasonDefaultOnly(l, catalog.ReportLintReasonDefaultOnlyParams{
			Locale: argString(c.ReasonArgs, "locale")})
	default:
		// Unmapped: print the raw code; lint_test.go fails on it.
		return string(c.Reason)
	}
}

// cmdLintDuplicates reports exact duplicate contracts, the unfiltered view of `lint shared`.
func cmdLintDuplicates(ctx context.Context, a *app, asJSON bool) error {
	cfg, set, err := loadAndReport(ctx, a)
	if err != nil {
		return err
	}

	var exact []shared.Candidate
	for _, c := range shared.Detect(cfg, set) {
		if c.Confidence == shared.High || c.Confidence == shared.Medium {
			exact = append(exact, c)
		}
	}

	if asJSON {
		enc := json.NewEncoder(a.data)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"duplicates": exact})
	}

	if len(exact) == 0 {
		a.says("loc: no duplicate contracts\n")
		return nil
	}
	for _, c := range exact {
		a.warns("%-10s %s\n", c.Confidence, strings.Join(c.Keys, "  "))
	}
	a.warns("\n%d duplicate contract(s)\n", len(exact))
	return nil
}

func sortedMapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// cmdLintKeys fails on a finding only under --strict, because a dynamic key looks unused.
func cmdLintKeys(ctx context.Context, a *app, asJSON bool) error {
	cfg, set, err := loadAndReport(ctx, a)
	if err != nil {
		return err
	}

	if len(cfg.Scan) == 0 {
		a.says("loc: no scan roots configured; nothing to compare against\n")
		return nil
	}

	refs, err := references(ctx, cfg, set)
	if err != nil {
		return err
	}

	declared := set.Keys()
	unused := scan.Unused(declared, refs, cfg.DynamicKeys)
	missing := scan.Missing(declared, refs)

	// One verdict for both forms, so --json exits as the human run does.
	var verdict error
	if a.opts.strict && len(missing)+len(unused) > 0 {
		verdict = errFailed
	}

	if asJSON {
		enc := json.NewEncoder(a.data)
		enc.SetIndent("", "  ")
		if err := enc.Encode(map[string]any{
			"filesScanned": refs.Files,
			"unused":       unused,
			"missing":      missing,
		}); err != nil {
			return err
		}
		return verdict
	}

	a.says("scanned %d file(s) in %d root(s)\n", refs.Files, len(cfg.Scan))

	// An undeclared key renders raw, so this class is always a bug.
	if len(missing) > 0 {
		a.warns("\nI18N030 referenced but not declared\n\n")
		for _, ref := range missing {
			a.warns("  %-40s %s:%d\n", ref.Key, ref.File, ref.Line)
		}
	}

	if len(unused) > 0 {
		a.warns("\nI18N032 declared but not referenced\n\n")
		for _, key := range unused {
			a.warns("  %s\n", key)
		}
		a.warns("\n  A key built at runtime looks unused. Declare it under dynamicKeys\n")
		a.warns("  rather than deleting it.\n")
	}

	if len(missing)+len(unused) == 0 {
		a.says("loc: every declared key is referenced, and every reference resolves\n")
	}
	return verdict
}

// references scans for key usages, mapping generated Go names back to keys.
func references(ctx context.Context, cfg *config.Config, set *message.Set) (*scan.Result, error) {
	if len(cfg.Scan) == 0 {
		return &scan.Result{Refs: map[string][]scan.Reference{}}, nil
	}

	extra, err := scan.Compile(cfg.KeyPatterns)
	if err != nil {
		return nil, err
	}

	// The output tree is skipped whole. A redirected target shares its package with hand-written code,
	// so it is walked and loc's files there are skipped by their generated header.
	return scan.Run(ctx, scan.Options{
		Roots:     cfg.AbsScan(),
		Generated: []string{cfg.AbsOutput()},
		GoNames:   generate.GoNames(cfg, set),
		Extra:     extra,
	})
}
