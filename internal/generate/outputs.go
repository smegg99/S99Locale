// internal/generate/outputs.go

package generate

import (
	"sort"

	"github.com/smegg99/s99locale/internal/message"
)

// Outputs is one staged tree per configured target, plus the retired trees to clear.
type Outputs struct {
	byTarget map[message.Target]*Output
	order    []message.Target
	// retired trees stage nothing, so writing one clears what its record names.
	retired []*Output
}

func newOutputs() *Outputs { return &Outputs{byTarget: map[message.Target]*Output{}} }

func (o *Outputs) add(out *Output) {
	o.byTarget[out.Target] = out
	o.order = append(o.order, out.Target)
}

// For is one target's tree, or nil when the config does not build that target.
func (o *Outputs) For(t message.Target) *Output { return o.byTarget[t] }

func (o *Outputs) all() []*Output {
	out := make([]*Output, 0, len(o.order)+len(o.retired))
	for _, t := range o.order {
		out = append(out, o.byTarget[t])
	}
	return append(out, o.retired...)
}

// Files is every staged file across every tree.
func (o *Outputs) Files() []File {
	var out []File
	for _, one := range o.all() {
		out = append(out, one.Files()...)
	}
	return out
}

// Write refuses if any tree holds a file loc did not provably write, then commits each tree.
func (o *Outputs) Write() (written int, removed []string, err error) {
	trees := o.all()
	plans := make([]plan, len(trees))
	var edited, foreign []string
	for i, one := range trees {
		p, err := one.plan()
		if err != nil {
			return 0, nil, err
		}
		plans[i] = p
		edited = append(edited, p.edited...)
		foreign = append(foreign, p.foreign...)
	}
	sort.Strings(edited)
	sort.Strings(foreign)
	if err := refusal(edited, foreign); err != nil {
		return 0, nil, err
	}

	// Each tree is atomic on its own; a later failure leaves earlier trees written.
	for i, one := range trees {
		w, r, err := one.commit(plans[i])
		written += w
		removed = append(removed, r...)
		if err != nil {
			return written, removed, err
		}
	}
	sort.Strings(removed)
	return written, removed, nil
}

// Diff reports every stale and hand-edited path across every tree, relative to the config.
func (o *Outputs) Diff() (stale, modified []string, err error) {
	for _, one := range o.all() {
		s, m, err := one.Diff()
		if err != nil {
			return nil, nil, err
		}
		stale = append(stale, s...)
		modified = append(modified, m...)
	}
	sort.Strings(stale)
	sort.Strings(modified)
	return stale, modified, nil
}
