// internal/shared/detect.go

package shared

import (
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
)

type Confidence string

const (
	// High means the whole contract matches in every locale.
	High Confidence = "high"
	// Medium means the text matches everywhere but the metadata disagrees.
	Medium Confidence = "medium"
	// Low means only the default locale matches.
	Low Confidence = "low"
)

// Reason names why a finding was downgraded; cmd/loc turns it into words.
type Reason string

const (
	ReasonOneLocale    Reason = "oneLocale"    // one locale configured, so matching text proves nothing
	ReasonDescriptions Reason = "descriptions" // the descriptions differ
	ReasonTargets      Reason = "targets"      // the namespaces go to different targets
	ReasonContract     Reason = "contract"     // the text matches everywhere, the contract does not
	ReasonDefaultOnly  Reason = "defaultOnly"  // only the default locale matches; args carry its locale
)

// Reasons is every Reason a Candidate can carry; a test checks cmd/loc renders each.
func Reasons() []Reason {
	return []Reason{ReasonOneLocale, ReasonDescriptions, ReasonTargets, ReasonContract, ReasonDefaultOnly}
}

// Candidate is a group of keys that may be one message.
type Candidate struct {
	Keys       []string   `json:"keys"`
	Confidence Confidence `json:"confidence"`
	// Suggested is empty when no reliable name can be derived.
	Suggested    string            `json:"suggested,omitempty"`
	Translations map[string]string `json:"translations"`
	// Reason explains a downgrade.
	Reason Reason `json:"reason,omitempty"`
	// ReasonArgs are the reason's placeholders, still typed.
	ReasonArgs map[string]any `json:"reasonArgs,omitempty"`
}

// Detect finds duplicate message contracts.
//
// Hashes index the search; equality is always confirmed by comparing the whole
// fingerprint, because a collision would merge two unrelated strings.
func Detect(cfg *config.Config, set *message.Set) []Candidate {
	prints := map[string]Fingerprint{}

	var keys []string
	for _, key := range set.Keys() {
		fp, ok := Of(set, key, cfg.Locales)
		if !ok {
			continue
		}
		prints[key] = fp
		keys = append(keys, key)
	}

	var out []Candidate
	out = append(out, exactGroups(cfg, set, keys, prints)...)
	out = append(out, textGroups(cfg, keys, prints, out)...)

	sort.Slice(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence {
			return rank(out[i].Confidence) < rank(out[j].Confidence)
		}
		return strings.Join(out[i].Keys, ",") < strings.Join(out[j].Keys, ",")
	})
	return out
}

func rank(c Confidence) int {
	switch c {
	case High:
		return 0
	case Medium:
		return 1
	default:
		return 2
	}
}

// exactGroups collects keys whose full contract matches in every locale.
func exactGroups(cfg *config.Config, set *message.Set, keys []string,
	prints map[string]Fingerprint) []Candidate {

	buckets := map[string][]string{}
	for _, key := range keys {
		h := prints[key].Hash()
		buckets[h] = append(buckets[h], key)
	}

	var out []Candidate
	for _, h := range sortedBucketKeys(buckets) {
		group := buckets[h]
		if len(group) < 2 {
			continue
		}

		// Confirm by full comparison; the hash only narrows the search.
		confirmed := confirm(group, prints)
		if len(confirmed) < 2 {
			continue
		}
		sort.Strings(confirmed)

		conf, reason := grade(cfg, set, confirmed)
		out = append(out, Candidate{
			Keys:         confirmed,
			Confidence:   conf,
			Reason:       reason,
			Suggested:    Suggest(cfg, set, confirmed),
			Translations: prints[confirmed[0]].Translations,
		})
	}
	return out
}

// confirm keeps only the members equal to the first.
func confirm(group []string, prints map[string]Fingerprint) []string {
	first := prints[group[0]]

	out := []string{group[0]}
	for _, key := range group[1:] {
		if first.Equal(prints[key]) {
			out = append(out, key)
		}
	}
	return out
}

// grade downgrades a match whose metadata disagrees.
func grade(cfg *config.Config, set *message.Set, group []string) (Confidence, Reason) {
	if len(cfg.Locales) < 2 {
		return Low, ReasonOneLocale
	}

	base, _ := set.Get(cfg.DefaultLocale, group[0])
	for _, key := range group[1:] {
		other, ok := set.Get(cfg.DefaultLocale, key)
		if !ok {
			continue
		}
		if base.Description != other.Description {
			return Medium, ReasonDescriptions
		}
		if !sameTargets(cfg, base, other) {
			return Medium, ReasonTargets
		}
	}
	return High, ""
}

func sameTargets(cfg *config.Config, a, b *message.Message) bool {
	left, right := cfg.TargetsFor(a.Namespace), cfg.TargetsFor(b.Namespace)
	if len(left) != len(right) {
		return false
	}

	seen := map[message.Target]bool{}
	for _, t := range left {
		seen[t] = true
	}
	for _, t := range right {
		if !seen[t] {
			return false
		}
	}
	return true
}

// textGroups finds keys whose text matches but whose contract does not.
//
// Two cases, graded apart because conflating them would mislabel both:
//
//   - matches in every locale, only the placeholder or plural contract differs
//   - matches in the default locale only, which says almost nothing: "Open"
//     the verb and "Open" the state are one word in English alone
func textGroups(cfg *config.Config, keys []string, prints map[string]Fingerprint,
	already []Candidate) []Candidate {

	if len(cfg.Locales) < 2 {
		return nil
	}

	reported := map[string]bool{}
	for _, c := range already {
		for _, key := range c.Keys {
			reported[key] = true
		}
	}

	buckets := map[string][]string{}
	for _, key := range keys {
		if reported[key] {
			continue
		}
		text := prints[key].Translations[cfg.DefaultLocale]
		buckets[text] = append(buckets[text], key)
	}

	var out []Candidate
	for _, text := range sortedBucketKeys(buckets) {
		group := buckets[text]
		if len(group) < 2 {
			continue
		}
		sort.Strings(group)

		conf, reason, args := gradeText(cfg, group, prints)
		out = append(out, Candidate{
			Keys:         group,
			Confidence:   conf,
			Reason:       reason,
			ReasonArgs:   args,
			Translations: prints[group[0]].Translations,
		})
	}
	return out
}

// gradeText separates "the same everywhere, different contract" from "the same
// in English only".
func gradeText(cfg *config.Config, group []string, prints map[string]Fingerprint) (Confidence, Reason, map[string]any) {
	first := prints[group[0]]

	everywhere := true
	for _, key := range group[1:] {
		for _, locale := range cfg.Locales {
			if !first.EqualIn(prints[key], locale) {
				everywhere = false
			}
		}
	}

	if everywhere {
		return Medium, ReasonContract, nil
	}
	return Low, ReasonDefaultOnly, map[string]any{"locale": cfg.DefaultLocale}
}

func sortedBucketKeys(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
