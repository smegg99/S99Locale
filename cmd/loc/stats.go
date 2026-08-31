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

	a.counts(catalog.ReportStatsByNamespace(l), byNamespace(set, keys, cfg.DefaultLocale))
	a.counts(catalog.ReportStatsByTarget(l), byTarget(cfg, set, keys))
	a.counts(catalog.ReportStatsMissing(l), missingCounts(cfg, set, keys))

	out, err := generate.Build(ctx, cfg, set)
	if err != nil {
		return err
	}
	a.sizes(cfg, out)
	return nil
}

// counts prints one titled count table, the shape all three stats tables share.
func (a *app) counts(title string, rows map[string]int) {
	l := a.say()

	built := make([][]string, 0, len(rows))
	for _, name := range sortedCountKeys(rows) {
		built = append(built, []string{name, strconv.Itoa(rows[name])})
	}

	a.warns(s99term.StatusNone, title)
	a.block(a.table([]string{catalog.ReportStatsColumnName(l), catalog.ReportStatsColumnCount(l)}, built))
}

func byNamespace(set *message.Set, keys []string, base string) map[string]int {
	counts := map[string]int{}
	for _, key := range keys {
		if m, ok := set.Get(base, key); ok {
			counts[m.Namespace]++
		}
	}
	return counts
}

func byTarget(cfg *config.Config, set *message.Set, keys []string) map[string]int {
	counts := map[string]int{}
	for _, target := range cfg.Targets {
		counts[string(target)] = 0
		for _, key := range keys {
			if m, ok := set.Get(cfg.DefaultLocale, key); ok && cfg.Wants(m, target) {
				counts[string(target)]++
			}
		}
	}
	return counts
}

func missingCounts(cfg *config.Config, set *message.Set, keys []string) map[string]int {
	counts := map[string]int{}
	for _, locale := range cfg.Locales {
		counts[locale] = 0
		for _, key := range keys {
			if _, ok := set.Get(locale, key); !ok {
				counts[locale]++
			}
		}
	}
	return counts
}

// sizes prints one row per target with its generated size, since each target is its own tree.
func (a *app) sizes(cfg *config.Config, out *generate.Outputs) {
	l := a.say()

	total := 0
	rows := make([][]string, 0, len(cfg.Targets)+1)
	for _, target := range cfg.Targets {
		size := 0
		for _, f := range out.For(target).Files() {
			size += len(f.Bytes)
		}
		total += size
		rows = append(rows, []string{string(target), humanBytes(size)})
	}
	rows = append(rows, []string{catalog.ReportStatsTotal(l), humanBytes(total)})

	a.warns(s99term.StatusNone, catalog.ReportStatsGeneratedSize(l))
	a.block(a.table([]string{catalog.ReportStatsColumnName(l), catalog.ReportStatsColumnSize(l)}, rows))
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
