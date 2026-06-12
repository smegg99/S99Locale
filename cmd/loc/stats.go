// cmd/loc/stats.go

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/message"
)

func cmdStats(ctx context.Context, path string) error {
	cfg, set, err := loadAndReport(ctx, path, false)
	if err != nil {
		return err
	}

	keys := set.Keys()
	fmt.Printf("locales   %v\n", cfg.Locales)
	fmt.Printf("messages  %d\n", len(keys))

	printByNamespace(set, keys, cfg.DefaultLocale)
	printByTarget(cfg, set, keys)
	printMissing(cfg, set, keys)

	out, err := generate.Build(ctx, cfg, set)
	if err != nil {
		return err
	}
	printSizes(out)
	return nil
}

func printByNamespace(set *message.Set, keys []string, base string) {
	counts := map[string]int{}
	for _, key := range keys {
		if m, ok := set.Get(base, key); ok {
			counts[m.Namespace]++
		}
	}

	fmt.Println("\nby namespace")
	for _, ns := range sortedCountKeys(counts) {
		fmt.Printf("  %-12s %d\n", ns, counts[ns])
	}
}

func printByTarget(cfg *config.Config, set *message.Set, keys []string) {
	fmt.Println("\nby target")
	for _, target := range cfg.Targets {
		count := 0
		for _, key := range keys {
			if m, ok := set.Get(cfg.DefaultLocale, key); ok && cfg.Wants(m, target) {
				count++
			}
		}
		fmt.Printf("  %-12s %d\n", target, count)
	}
}

func printMissing(cfg *config.Config, set *message.Set, keys []string) {
	fmt.Println("\nmissing translations")
	for _, locale := range cfg.Locales {
		missing := 0
		for _, key := range keys {
			if _, ok := set.Get(locale, key); !ok {
				missing++
			}
		}
		fmt.Printf("  %-12s %d\n", locale, missing)
	}
}

func printSizes(out *generate.Outputs) {
	fmt.Println("\ngenerated size")

	total := 0
	byTop := map[string]int{}
	for _, f := range out.Files() {
		total += len(f.Bytes)
		byTop[topDir(f.Path)] += len(f.Bytes)
	}

	for _, dir := range sortedCountKeys(byTop) {
		fmt.Printf("  %-12s %s\n", dir, humanBytes(byTop[dir]))
	}
	fmt.Printf("  %-12s %s\n", "total", humanBytes(total))
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
