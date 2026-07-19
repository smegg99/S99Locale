// cmd/loc/stats.go

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
)

func newStatsCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Counts by namespace, target and locale, plus bundle sizes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdStats(cmd.Context(), a)
		},
	}
}

func cmdStats(ctx context.Context, a *app) error {
	cfg, set, err := loadAndReport(ctx, a)
	if err != nil {
		return err
	}

	keys := set.Keys()
	a.warns("locales   %v\n", cfg.Locales)
	a.warns("messages  %d\n", len(keys))

	printByNamespace(a, set, keys, cfg.DefaultLocale)
	printByTarget(a, cfg, set, keys)
	printMissing(a, cfg, set, keys)

	out, err := generate.Build(ctx, cfg, set)
	if err != nil {
		return err
	}
	printSizes(a, out)
	return nil
}

func printByNamespace(a *app, set *message.Set, keys []string, base string) {
	counts := map[string]int{}
	for _, key := range keys {
		if m, ok := set.Get(base, key); ok {
			counts[m.Namespace]++
		}
	}

	a.warns("\nby namespace\n")
	for _, ns := range sortedCountKeys(counts) {
		a.warns("  %-12s %d\n", ns, counts[ns])
	}
}

func printByTarget(a *app, cfg *config.Config, set *message.Set, keys []string) {
	a.warns("\nby target\n")
	for _, target := range cfg.Targets {
		count := 0
		for _, key := range keys {
			if m, ok := set.Get(cfg.DefaultLocale, key); ok && cfg.Wants(m, target) {
				count++
			}
		}
		a.warns("  %-12s %d\n", target, count)
	}
}

func printMissing(a *app, cfg *config.Config, set *message.Set, keys []string) {
	a.warns("\nmissing translations\n")
	for _, locale := range cfg.Locales {
		missing := 0
		for _, key := range keys {
			if _, ok := set.Get(locale, key); !ok {
				missing++
			}
		}
		a.warns("  %-12s %d\n", locale, missing)
	}
}

func printSizes(a *app, out *generate.Outputs) {
	a.warns("\ngenerated size\n")

	total := 0
	byTop := map[string]int{}
	for _, f := range out.Files() {
		total += len(f.Bytes)
		byTop[topDir(f.Path)] += len(f.Bytes)
	}

	for _, dir := range sortedCountKeys(byTop) {
		a.warns("  %-12s %s\n", dir, humanBytes(byTop[dir]))
	}
	a.warns("  %-12s %s\n", "total", humanBytes(total))
}

func topDir(p string) string {
	if i := strings.IndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return "."
}

func humanBytes(n int) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f kB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func sortedCountKeys(in map[string]int) []string {
	out := make([]string, 0, len(in))
	for k := range in {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
