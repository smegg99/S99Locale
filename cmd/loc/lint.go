// cmd/loc/lint.go

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/smegg99/s99term"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/scan"
	"github.com/smegg99/s99locale/internal/shared"
	"github.com/smegg99/s99locale/internal/validate"
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
		a.says(s99term.StatusOK, catalog.ReportLintNoShared(a.say()))
		if hidden := len(all) - len(shown); hidden > 0 {
			a.says(s99term.StatusWarning, catalog.ReportLintHidden(a.say(), catalog.ReportLintHiddenParams{
				Count: hidden}))
		}
		return nil
	}

	for _, c := range shown {
		a.printCandidate(c)
	}

	a.warns(s99term.StatusWarning, catalog.ReportLintCandidates(a.say(), catalog.ReportLintCandidatesParams{
		Count: len(shown)}))
	if hidden := len(all) - len(shown); hidden > 0 {
		a.warns(s99term.StatusWarning, catalog.ReportLintHidden(a.say(), catalog.ReportLintHiddenParams{
			Count: hidden}))
	}
	return nil
}

// printCandidate composes one finding as three label blocks.
func (a *app) printCandidate(c shared.Candidate) {
	l := a.say()

	a.warns(s99term.StatusWarning, catalog.ReportLintPossibleShared(l),
		s99term.F("code", validate.CodePossibleShared))

	a.block(a.pairs([][2]string{{catalog.ReportLintKeysLabel(l), strings.Join(c.Keys, "\n")}}))

	rows := [][2]string{{catalog.ReportLintTranslationsLabel(l), ""}}
	for _, locale := range sortedMapKeys(c.Translations) {
		// A plural's forms are joined by a NUL; show the separator instead.
		rows = append(rows, [2]string{locale, strings.ReplaceAll(c.Translations[locale], "\x00", " | ")})
	}
	a.block(a.pairs(rows))

	tail := [][2]string{}
	if c.Suggested != "" {
		tail = append(tail, [2]string{catalog.ReportLintSuggestedLabel(l), c.Suggested})
	}
	tail = append(tail, [2]string{catalog.ReportLintConfidenceLabel(l), confidenceText(l, c.Confidence)})
	if c.Reason != "" {
		tail = append(tail, [2]string{"", reasonText(l, c)})
	}
	a.block(a.pairs(tail))
}

// confidenceText is a confidence in words; --json keeps the stable word.
func confidenceText(l *i18n.Localizer, c shared.Confidence) string {
	switch c {
	case shared.High:
		return catalog.ReportLintConfidenceHigh(l)
	case shared.Medium:
		return catalog.ReportLintConfidenceMedium(l)
	case shared.Low:
		return catalog.ReportLintConfidenceLow(l)
	default:
		// Unmapped: print the raw word; lint_test.go fails on it.
		return string(c)
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
		a.says(s99term.StatusOK, catalog.ReportLintNoDuplicates(a.say()))
		return nil
	}
	l := a.say()
	rows := make([][]string, 0, len(exact))
	for _, c := range exact {
		rows = append(rows, []string{confidenceText(l, c.Confidence), strings.Join(c.Keys, "  ")})
	}
	a.block(a.table([]string{catalog.ReportLintColumnConfidence(l), catalog.ReportLintColumnKeys(l)}, rows))
	a.warns(s99term.StatusWarning, catalog.ReportLintDuplicates(a.say(), catalog.ReportLintDuplicatesParams{
		Count: len(exact)}))
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
		a.says(s99term.StatusSkipped, catalog.ReportLintNoScanRoots(a.say()))
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

	l := a.say()
	a.says(s99term.StatusRunning, catalog.ReportLintScanned(l, catalog.ReportLintScannedParams{
		Files: refs.Files, Roots: len(cfg.Scan)}))

	// An undeclared key renders raw, so this class is always a bug.
	if len(missing) > 0 {
		a.warns(s99term.StatusFailed, catalog.ReportLintReferencedNotDeclared(l),
			s99term.F("code", validate.CodeReferencedNotDeclared))
		a.block(a.table(
			[]string{catalog.ReportLintColumnKey(l), catalog.ReportLintColumnLocation(l)},
			referenceRows(missing)))
	}

	if len(unused) > 0 {
		a.warns(s99term.StatusWarning, catalog.ReportLintDeclaredNotReferenced(l),
			s99term.F("code", validate.CodeDeclaredNotReferenced))

		rows := make([][]string, 0, len(unused))
		for _, key := range unused {
			rows = append(rows, []string{key})
		}
		a.block(a.table([]string{catalog.ReportLintColumnKey(l)}, rows))
		a.warns(s99term.StatusNone, catalog.ReportLintDynamicHint(l))
	}

	if len(missing)+len(unused) == 0 {
		a.says(s99term.StatusOK, catalog.ReportLintClean(l))
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

func referenceRows(refs []scan.Reference) [][]string {
	rows := make([][]string, 0, len(refs))
	for _, ref := range refs {
		rows = append(rows, []string{ref.Key, fmt.Sprintf("%s:%d", ref.File, ref.Line)})
	}
	return rows
}
