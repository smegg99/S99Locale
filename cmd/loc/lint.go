// cmd/loc/lint.go

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/scan"
	"github.com/smegg99/s99locale/internal/shared"
)

// cmdLintShared reports keys that may be one message.
//
// Only high confidence is shown by default; medium and low are right often
// enough to keep but wrong often enough to hide behind --verbose.
func cmdLintShared(ctx context.Context, configPath string, verbose, asJSON bool) error {
	cfg, set, err := loadAndReport(ctx, configPath, false)
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"candidates": shown})
	}

	if len(shown) == 0 {
		fmt.Println("loc: no shared-message candidates")
		if hidden := len(all) - len(shown); hidden > 0 {
			fmt.Printf("     %d lower-confidence finding(s); see --verbose\n", hidden)
		}
		return nil
	}

	for _, c := range shown {
		printCandidate(c)
	}

	fmt.Printf("%d candidate(s)\n", len(shown))
	if hidden := len(all) - len(shown); hidden > 0 {
		fmt.Printf("%d lower-confidence finding(s) hidden; see --verbose\n", hidden)
	}
	return nil
}

func printCandidate(c shared.Candidate) {
	fmt.Printf("\nI18N021 possible shared message\n\n")

	fmt.Println("  Keys:")
	for _, key := range c.Keys {
		fmt.Printf("    %s\n", key)
	}

	fmt.Println("\n  Translations:")
	for _, locale := range sortedMapKeys(c.Translations) {
		// A plural's forms are joined by a NUL; show the separator instead.
		text := strings.ReplaceAll(c.Translations[locale], "\x00", " | ")
		fmt.Printf("    %-4s %s\n", locale+":", text)
	}

	if c.Suggested != "" {
		fmt.Printf("\n  Suggested key:\n    %s\n", c.Suggested)
	}
	fmt.Printf("\n  Confidence:\n    %s\n", c.Confidence)
	if c.Reason != "" {
		fmt.Printf("    %s\n", c.Reason)
	}
}

// cmdLintDuplicates reports exact duplicate contracts, the unfiltered view of `lint shared`.
func cmdLintDuplicates(ctx context.Context, configPath string, asJSON bool) error {
	cfg, set, err := loadAndReport(ctx, configPath, false)
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
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{"duplicates": exact})
	}

	if len(exact) == 0 {
		fmt.Println("loc: no duplicate contracts")
		return nil
	}
	for _, c := range exact {
		fmt.Printf("%-10s %s\n", c.Confidence, strings.Join(c.Keys, "  "))
	}
	fmt.Printf("\n%d duplicate contract(s)\n", len(exact))
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

// cmdLintKeys reports what the source scan and the declarations disagree about.
//
// Dynamic keys exist, so an unused key is a warning and never a deletion.
// Nothing here fails the build.
func cmdLintKeys(ctx context.Context, configPath string, asJSON bool) error {
	cfg, set, err := loadAndReport(ctx, configPath, false)
	if err != nil {
		return err
	}

	if len(cfg.Scan) == 0 {
		fmt.Println("loc: no scan roots configured; nothing to compare against")
		return nil
	}

	refs, err := references(cfg, set)
	if err != nil {
		return err
	}

	declared := set.Keys()
	unused := scan.Unused(declared, refs, cfg.DynamicKeys)
	missing := scan.Missing(declared, refs)
	deprecated := deprecatedInUse(cfg, set, refs)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"filesScanned": refs.Files,
			"unused":       unused,
			"missing":      missing,
			"deprecated":   deprecated,
		})
	}

	fmt.Printf("scanned %d file(s) in %d root(s)\n", refs.Files, len(cfg.Scan))

	// A reference to a key nobody declares renders the raw key, so this is the
	// one of the three that is always a bug.
	if len(missing) > 0 {
		fmt.Printf("\nI18N030 referenced but not declared\n\n")
		for _, ref := range missing {
			fmt.Printf("  %-40s %s:%d\n", ref.Key, ref.File, ref.Line)
		}
	}

	if len(deprecated) > 0 {
		fmt.Printf("\nI18N031 deprecated key still referenced\n\n")
		for _, ref := range deprecated {
			fmt.Printf("  %-40s %s:%d\n", ref.Key, ref.File, ref.Line)
		}
	}

	if len(unused) > 0 {
		fmt.Printf("\nI18N032 declared but not referenced\n\n")
		for _, key := range unused {
			fmt.Printf("  %s\n", key)
		}
		fmt.Println("\n  A key built at runtime looks unused. Declare it under dynamicKeys")
		fmt.Println("  rather than deleting it.")
	}

	if len(missing)+len(deprecated)+len(unused) == 0 {
		fmt.Println("loc: every declared key is referenced, and every reference resolves")
	}
	return nil
}

// deprecatedInUse finds references to keys marked deprecated.
func deprecatedInUse(cfg *config.Config, set *message.Set, refs *scan.Result) []scan.Reference {
	var out []scan.Reference

	for _, key := range set.Keys() {
		m, ok := set.Get(cfg.DefaultLocale, key)
		if !ok || !m.Deprecated {
			continue
		}
		out = append(out, refs.Refs[key]...)
	}
	return out
}
