// internal/generate/outputs.go

package generate

import (
	"path/filepath"
	"sort"
)

// Outputs is a build's staged trees. There is normally one; a config that
// places the Go target in the package that embeds it has a second, which loc
// shares with that package's own source.
type Outputs struct {
	Primary *Output
	Go      *Output
}

func (o *Outputs) all() []*Output {
	if o.Go == nil {
		return []*Output{o.Primary}
	}
	return []*Output{o.Primary, o.Go}
}

// Files is every staged file across every tree.
func (o *Outputs) Files() []File {
	var out []File
	for _, one := range o.all() {
		out = append(out, one.Files()...)
	}
	return out
}

// Write commits every tree. A failure leaves the trees already written in
// place: they are each atomic on their own, and the caller is told which step
// failed rather than left with a half-reverted build.
func (o *Outputs) Write() (written, removed int, err error) {
	for _, one := range o.all() {
		w, r, err := one.Write()
		written += w
		removed += r
		if err != nil {
			return written, removed, err
		}
	}
	return written, removed, nil
}

// Diff reports every stale path across every tree. A second tree's paths carry
// its directory, so a report names a file somebody can open.
func (o *Outputs) Diff() ([]string, error) {
	var stale []string
	for _, one := range o.all() {
		found, err := one.Diff()
		if err != nil {
			return nil, err
		}
		for _, rel := range found {
			if one == o.Primary {
				stale = append(stale, rel)
				continue
			}
			stale = append(stale, filepath.Join(one.Dir, rel))
		}
	}
	sort.Strings(stale)
	return stale, nil
}
