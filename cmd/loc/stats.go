// cmd/loc/stats.go

package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/smegg99/s99term"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
)

func newStatsCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:  "stats",
		Args: a.noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdStats(cmd.Context(), a)
		},
	}

	a.describe(func(l *i18n.Localizer) { cmd.Short = catalog.CliStatsShort(l) })
	return cmd
}

func cmdStats(ctx context.Context, a *app) error {
	cfg, set, err := loadAndReport(ctx, a)
	if err != nil {
		return err
	}

	l := a.say()

	// The report is the command's result, not a success line, so it survives --quiet.
	keys := set.Keys()
	a.warns(s99term.StatusNone, catalog.ReportStatsLocales(l), s99term.F("locales", strings.Join(cfg.Locales, ", ")))
	a.warns(s99term.StatusNone, catalog.ReportStatsMessages(l), s99term.F("count", strconv.Itoa(len(keys))))

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

	a.warns(s99term.StatusNone, catalog.ReportStatsByNamespace(a.say()))
	for _, ns := range sortedCountKeys(counts) {
		a.warns(s99term.StatusNone, ns, s99term.F("count", strconv.Itoa(counts[ns])))
	}
}

func printByTarget(a *app, cfg *config.Config, set *message.Set, keys []string) {
	a.warns(s99term.StatusNone, catalog.ReportStatsByTarget(a.say()))
	for _, target := range cfg.Targets {
		count := 0
		for _, key := range keys {
			if m, ok := set.Get(cfg.DefaultLocale, key); ok && cfg.Wants(m, target) {
				count++
			}
		}
		a.warns(s99term.StatusNone, string(target), s99term.F("count", strconv.Itoa(count)))
	}
}

func printMissing(a *app, cfg *config.Config, set *message.Set, keys []string) {
	a.warns(s99term.StatusNone, catalog.ReportStatsMissing(a.say()))
	for _, locale := range cfg.Locales {
		missing := 0
		for _, key := range keys {
			if _, ok := set.Get(locale, key); !ok {
				missing++
			}
		}
		a.warns(s99term.StatusNone, locale, s99term.F("count", strconv.Itoa(missing)))
	}
}

// printSizes has one row per target, because each target is its own tree.
func printSizes(a *app, cfg *config.Config, out *generate.Outputs) {
	a.warns(s99term.StatusNone, catalog.ReportStatsGeneratedSize(a.say()))

	total := 0
	for _, target := range cfg.Targets {
		size := 0
		for _, f := range out.For(target).Files() {
			size += len(f.Bytes)
		}
		total += size
		a.warns(s99term.StatusNone, string(target), s99term.F("size", humanBytes(size)))
	}
	a.warns(s99term.StatusNone, catalog.ReportStatsTotal(a.say()), s99term.F("size", humanBytes(total)))
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
