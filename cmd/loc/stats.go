// cmd/loc/stats.go

package main

import (
	"context"
	"fmt"
	"sort"

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
	printSizes(a, cfg, out)
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

// printSizes has one row per target, because each target is its own tree.
func printSizes(a *app, cfg *config.Config, out *generate.Outputs) {
	a.warns("\ngenerated size\n")

	total := 0
	for _, target := range cfg.Targets {
		size := 0
		for _, f := range out.For(target).Files() {
			size += len(f.Bytes)
		}
		total += size
		a.warns("  %-12s %s\n", target, humanBytes(size))
	}
	a.warns("  %-12s %s\n", "total", humanBytes(total))
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
