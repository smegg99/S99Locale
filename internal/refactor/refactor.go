// internal/refactor/refactor.go

// Package refactor plans a key rename. It never writes anything.
//
// A rename touches canonical YAML in every locale and every call site in two
// languages, and a missed reference renders the raw key. So this produces a
// patch for review and refuses whenever it cannot see every reference.
package refactor

import (
	"fmt"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/scan"
	"github.com/smegg99/s99locale/internal/shared"
)

// Plan is what a refactor would do.
type Plan struct {
	From []string
	To   string
	// Edits are the source changes, sorted by file then line.
	Edits []Edit
	// Notes are things a human has to handle: this never writes, so they are
	// instructions rather than warnings.
	Notes []string
}

// Edit is one line to change.
type Edit struct {
	File string
	Line int
	Old  string
	New  string
}

// Refusal is why a refactor cannot be planned. Separate from an ordinary error
// so the caller can tell a declined request from a failed one.
type Refusal struct{ Reason string }

func (r *Refusal) Error() string { return "refactor refused: " + r.Reason }

// PlanRename checks the request and builds the patch.
//
// Refuses when the contracts differ, when the destination exists with a
// different contract, or when a source key is reachable dynamically - each
// would change behaviour in a way the patch could not show.
func PlanRename(cfg *config.Config, set *message.Set, refs *scan.Result,
	from []string, to string) (*Plan, error) {

	if len(from) < 1 {
		return nil, &Refusal{"nothing to rename"}
	}
	if to == "" {
		return nil, &Refusal{"no destination key"}
	}

	if err := checkContracts(cfg, set, from); err != nil {
		return nil, err
	}
	if err := checkDestination(cfg, set, from, to); err != nil {
		return nil, err
	}
	if err := checkDynamic(cfg, from); err != nil {
		return nil, err
	}

	plan := &Plan{From: append([]string(nil), from...), To: to}
	sort.Strings(plan.From)

	for _, key := range plan.From {
		if key == to {
			continue
		}
		for _, ref := range refs.Refs[key] {
			plan.Edits = append(plan.Edits, Edit{
				File: ref.File,
				Line: ref.Line,
				Old:  key,
				New:  to,
			})
		}
	}
	sort.Slice(plan.Edits, func(i, j int) bool {
		if plan.Edits[i].File != plan.Edits[j].File {
			return plan.Edits[i].File < plan.Edits[j].File
		}
		return plan.Edits[i].Line < plan.Edits[j].Line
	})

	plan.Notes = notes(cfg, set, plan)
	return plan, nil
}

// checkContracts refuses a merge of messages that are not the same message.
func checkContracts(cfg *config.Config, set *message.Set, from []string) error {
	first, ok := shared.Of(set, from[0], cfg.Locales)
	if !ok {
		return &Refusal{fmt.Sprintf("%s is missing from at least one locale", from[0])}
	}

	for _, key := range from[1:] {
		other, ok := shared.Of(set, key, cfg.Locales)
		if !ok {
			return &Refusal{fmt.Sprintf("%s is missing from at least one locale", key)}
		}
		if !first.Equal(other) {
			return &Refusal{fmt.Sprintf(
				"%s and %s are not the same message; merging them would change one of them",
				from[0], key)}
		}
	}
	return nil
}

// checkDestination allows merging into an existing key only when it already
// says exactly the same thing.
func checkDestination(cfg *config.Config, set *message.Set, from []string, to string) error {
	dest, ok := shared.Of(set, to, cfg.Locales)
	if !ok {
		// Does not exist yet, or is incomplete. Creating it is fine; an
		// incomplete one is validation's problem, not this command's.
		return nil
	}

	source, ok := shared.Of(set, from[0], cfg.Locales)
	if !ok {
		return &Refusal{fmt.Sprintf("%s is missing from at least one locale", from[0])}
	}
	if !dest.Equal(source) {
		return &Refusal{fmt.Sprintf(
			"%s already exists and says something different; pick another name or reconcile them first", to)}
	}
	return nil
}

// checkDynamic refuses when a key could also be built at runtime, because a
// scan cannot see those references and the patch would miss them.
func checkDynamic(cfg *config.Config, from []string) error {
	for _, key := range from {
		for _, d := range cfg.DynamicKeys {
			if d == key || (strings.HasSuffix(d, ".") && strings.HasPrefix(key, d)) {
				return &Refusal{fmt.Sprintf(
					"%s is declared as a dynamic key, so its references cannot all be found", key)}
			}
		}
	}
	return nil
}

func notes(cfg *config.Config, set *message.Set, plan *Plan) []string {
	var out []string

	for _, key := range plan.From {
		if key == plan.To {
			continue
		}
		for _, locale := range cfg.Locales {
			if m, ok := set.Get(locale, key); ok {
				out = append(out, fmt.Sprintf("remove %s from %s", key, m.Source.File))
			}
		}
	}

	if _, exists := set.Get(cfg.DefaultLocale, plan.To); !exists {
		out = append(out, fmt.Sprintf(
			"add %s to each locale, with the text %s already has", plan.To, plan.From[0]))
	}

	if len(plan.Edits) == 0 {
		out = append(out, "no source references found; either nothing uses these keys "+
			"or the scan roots are not configured")
	}
	return out
}

// Patch renders the plan as a unified diff a human can read and apply.
func (p *Plan) Patch() string {
	if len(p.Edits) == 0 {
		return ""
	}

	var b strings.Builder
	current := ""

	for _, e := range p.Edits {
		if e.File != current {
			current = e.File
			fmt.Fprintf(&b, "--- a/%s\n+++ b/%s\n", current, current)
		}
		fmt.Fprintf(&b, "@@ line %d @@\n-%s\n+%s\n", e.Line, e.Old, e.New)
	}
	return b.String()
}
