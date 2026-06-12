// internal/generate/write.go

// Package generate turns the neutral AST into per-target bundles.
//
// Every writer here is deterministic and atomic: two builds on the same input
// produce identical bytes, and a build that dies halfway leaves no truncated
// file behind.
package generate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// File is one generated artefact, held in memory until the whole build succeeds.
type File struct {
	Path  string
	Bytes []byte
}

// Output collects files and writes them in one pass.
type Output struct {
	Dir   string
	files []File
	fs    fileSystem

	// owns reports whether a path under Dir is loc's to remove. Nil means the
	// whole tree is, which is true of the output directory. A Go package that
	// holds hand-written source beside the generated bundle supplies one.
	owns func(rel string) bool
}

func NewOutput(dir string) *Output { return &Output{Dir: dir, fs: osFS{}} }

// NewSharedOutput is an output in a directory loc does not own outright. Only
// paths the predicate accepts are ever removed; everything else is left alone,
// and the directory itself is never pruned.
func NewSharedOutput(dir string, owns func(rel string) bool) *Output {
	return &Output{Dir: dir, fs: osFS{}, owns: owns}
}

func (o *Output) Add(rel string, body []byte) {
	o.files = append(o.files, File{Path: rel, Bytes: body})
}

// AddJSON writes minified JSON with sorted keys.
func (o *Output) AddJSON(rel string, v any) error {
	body, err := marshal(v)
	if err != nil {
		return fmt.Errorf("generate: %s: %w", rel, err)
	}
	o.Add(rel, body)
	return nil
}

// Files returns everything staged, sorted by path.
func (o *Output) Files() []File {
	sort.Slice(o.files, func(i, j int) bool { return o.files[i].Path < o.files[j].Path })
	return o.files
}

// Write commits every staged file and removes anything else under Dir.
// Unchanged files are left alone so a watcher downstream does not rebuild the
// world over a byte-identical write.
//
// Two phases. Everything is staged as a temp file first, and only once every
// one of them exists does anything become visible. The realistic failure - a
// full disk, a read-only tree - lands in the staging phase, where nothing has
// been committed and nothing has been deleted. Renaming is what remains, and it
// is orders of magnitude less likely to fail partway.
func (o *Output) Write() (written, removed int, err error) {
	staged, err := o.prepare()
	if err != nil {
		// Nothing is visible yet, so the tree is exactly as it was.
		o.discard(staged)
		return 0, 0, err
	}

	for _, s := range staged {
		if err := o.fs.Rename(s.temp, s.dest); err != nil {
			return written, 0, err
		}
		written++
	}

	orphans, err := o.orphans()
	if err != nil {
		return written, removed, err
	}
	for _, rel := range orphans {
		if err := o.fs.Remove(filepath.Join(o.Dir, rel)); err != nil {
			return written, removed, err
		}
		removed++
	}
	if o.owns != nil {
		// Not loc's directory to tidy: an empty `locales/` is still the app's.
		return written, removed, nil
	}
	return written, removed, pruneEmptyDirs(o.Dir)
}

// staging is one file written but not yet visible.
type staging struct {
	temp string
	dest string
}

// prepare writes every changed file beside its destination. Unchanged files are
// skipped entirely, which is what keeps a downstream watcher quiet.
func (o *Output) prepare() ([]staging, error) {
	var staged []staging

	for _, f := range o.Files() {
		dest := filepath.Join(o.Dir, f.Path)

		same, err := unchanged(dest, f.Bytes)
		if err != nil {
			return staged, err
		}
		if same {
			continue
		}

		dir := filepath.Dir(dest)
		if err := o.fs.MkdirAll(dir); err != nil {
			return staged, err
		}
		temp, err := o.fs.WriteTemp(dir, f.Bytes)
		if err != nil {
			return staged, err
		}
		staged = append(staged, staging{temp: temp, dest: dest})
	}
	return staged, nil
}

// discard removes staged files after a failure, ignoring errors so the original one is returned.
func (o *Output) discard(staged []staging) {
	for _, s := range staged {
		_ = o.fs.Remove(s.temp)
	}
}

// Diff reports paths whose contents differ from what is on disk, paths that are
// missing, and paths that should no longer exist. This is what `loc check`
// reads.
func (o *Output) Diff() ([]string, error) {
	var stale []string

	for _, f := range o.Files() {
		same, err := unchanged(filepath.Join(o.Dir, f.Path), f.Bytes)
		if err != nil {
			return nil, err
		}
		if !same {
			stale = append(stale, f.Path)
		}
	}

	orphans, err := o.orphans()
	if err != nil {
		return nil, err
	}
	stale = append(stale, orphans...)
	sort.Strings(stale)
	return stale, nil
}

// orphans lists files under Dir that this build did not produce.
//
// Everything under the output directory belongs to loc - `loc clean` deletes
// the whole tree - so anything left behind is output from a config that no
// longer holds. Leaving it is not harmless: switching pseudolocale off left a
// full locale on disk, an app's `import.meta.glob` swept it back up, and a
// language nobody generates reappeared in the language menu.
func (o *Output) orphans() ([]string, error) {
	want := make(map[string]struct{}, len(o.files))
	for _, f := range o.Files() {
		want[filepath.Clean(f.Path)] = struct{}{}
	}

	var extra []string
	err := filepath.WalkDir(o.Dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			// A missing output directory is the first build, not a failure.
			if os.IsNotExist(err) && path == o.Dir {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(o.Dir, path)
		if err != nil {
			return err
		}
		// A .loc-* file is another run staging its output, not debris. The
		// lock should prevent one being visible here at all; skipping it means
		// a platform without locking degrades to leaving a stray temp file
		// rather than deleting a file mid-write.
		if strings.HasPrefix(filepath.Base(rel), tempPrefix) {
			return nil
		}
		if _, ok := want[rel]; ok {
			return nil
		}
		if o.owns != nil && !o.owns(rel) {
			return nil
		}
		extra = append(extra, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("generate: scan %s: %w", o.Dir, err)
	}
	sort.Strings(extra)
	return extra, nil
}

// pruneEmptyDirs removes directories left behind once their files are gone,
// deepest first. Dir itself stays, so a build that produces nothing still has
// somewhere to write next time.
func pruneEmptyDirs(root string) error {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) && path == root {
				return filepath.SkipAll
			}
			return err
		}
		if d.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("generate: scan %s: %w", root, err)
	}

	// Deepest first, so a directory holding only empty directories also goes.
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return fmt.Errorf("generate: read %s: %w", dir, err)
		}
		if len(entries) > 0 {
			continue
		}
		if err := os.Remove(dir); err != nil {
			return fmt.Errorf("generate: remove %s: %w", dir, err)
		}
	}
	return nil
}

func unchanged(path string, want []byte) (bool, error) {
	got, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("generate: read %s: %w", path, err)
	}
	return bytes.Equal(got, want), nil
}

// marshal produces minified JSON; encoding/json sorts map keys, so output is stable.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Keep the trailing newline Encode appends; editors add one back otherwise.
	return buf.Bytes(), nil
}
