// internal/generate/write.go

// Package generate turns the neutral AST into deterministic, atomically written per-target bundles.
package generate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
)

// File is one generated artefact, held in memory until the whole build succeeds.
type File struct {
	Path  string
	Bytes []byte
}

// Output collects one target's files and writes them in one pass.
type Output struct {
	Dir string
	// Label is Dir as a user would type it, for messages.
	Label  string
	Target message.Target

	files []File
	fs    fileSystem
}

// NewOutput is an empty tree for one target, written into dir and named by label.
func NewOutput(dir, label string, target message.Target) *Output {
	return &Output{Dir: dir, Label: label, Target: target, fs: osFS{}}
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

// Write commits every staged file, then removes recorded files this build does not produce.
func (o *Output) Write() (written int, removed []string, err error) {
	p, err := o.plan()
	if err != nil {
		return 0, nil, err
	}
	if err := refusal(p.edited, p.foreign); err != nil {
		return 0, nil, err
	}
	return o.commit(p)
}

// commit writes the staged files and carries out a plan that nothing refused.
func (o *Output) commit(p plan) (written int, removed []string, err error) {
	staged, record, err := o.prepare()
	if record != nil {
		// Any failure before the record's own rename discards it, so the old record stays.
		defer func() {
			if err != nil {
				o.discard([]staging{*record})
			}
		}()
	}
	if err != nil {
		o.discard(staged)
		return 0, nil, err
	}

	// No context check here: a cancellation between renames would leave a half-committed tree.
	for i, s := range staged {
		if err := o.fs.Rename(s.temp, s.dest); err != nil {
			o.discard(staged[i:])
			return written, removed, err
		}
		written++
	}

	var emptied []string
	for _, rel := range p.remove {
		full := filepath.Join(o.Dir, filepath.FromSlash(rel))
		if err := o.fs.Remove(full); err != nil {
			return written, removed, err
		}
		emptied = append(emptied, filepath.Dir(full))
		removed = append(removed, path.Join(o.Label, rel))
	}

	// The record is renamed or removed last, so a rerun on the same sources finishes the removals.
	// A crash before that leaves new files unrecorded and rewritten files under their old hash.
	if p.dropRecord {
		if err := o.fs.Remove(filepath.Join(o.Dir, ManifestName)); err != nil {
			return written, removed, err
		}
		removed = append(removed, path.Join(o.Label, ManifestName))
	}
	if err := pruneEmptied(o.Dir, emptied); err != nil {
		return written, removed, err
	}
	if p.dropRecord {
		// Nothing is staged here, so an empty directory is loc's own leftover.
		return written, removed, removeIfEmpty(o.Dir)
	}

	if record != nil {
		if err := o.fs.Rename(record.temp, record.dest); err != nil {
			return written, removed, err
		}
		written++
	}
	return written, removed, nil
}

// staging is one file written but not yet visible.
type staging struct {
	temp string
	dest string
}

// prepare writes every changed file beside its destination and returns the record separately.
func (o *Output) prepare() ([]staging, *staging, error) {
	// Unchanged files are skipped so a downstream watcher stays quiet.
	// record is nil when its bytes are unchanged.
	var staged []staging
	var record *staging

	for _, f := range o.Files() {
		dest := filepath.Join(o.Dir, filepath.FromSlash(f.Path))

		same, err := unchanged(dest, f.Bytes)
		if err != nil {
			return staged, record, err
		}
		if same {
			continue
		}

		dir := filepath.Dir(dest)
		if err := o.fs.MkdirAll(dir); err != nil {
			return staged, record, err
		}
		temp, err := o.fs.WriteTemp(dir, f.Bytes)
		if err != nil {
			return staged, record, err
		}

		one := staging{temp: temp, dest: dest}
		if f.Path == ManifestName {
			record = &one
			continue
		}
		staged = append(staged, one)
	}
	return staged, record, nil
}

// discard removes staged files after a failure, ignoring errors so the original one is returned.
func (o *Output) discard(staged []staging) {
	for _, s := range staged {
		_ = o.fs.Remove(s.temp)
	}
}

// Diff reports stale paths `loc build` fixes and modified paths it refuses to touch.
func (o *Output) Diff() (stale, modified []string, err error) {
	// Stale: differing, missing, or recorded but not produced.
	// Modified: edited after loc wrote it, or about to be overwritten without a record.
	p, err := o.plan()
	if err != nil {
		return nil, nil, err
	}

	for _, f := range o.Files() {
		label := path.Join(o.Label, f.Path)
		if slices.Contains(p.foreign, label) {
			continue
		}
		same, err := unchanged(filepath.Join(o.Dir, filepath.FromSlash(f.Path)), f.Bytes)
		if err != nil {
			return nil, nil, err
		}
		if !same {
			stale = append(stale, label)
		}
	}
	for _, rel := range p.remove {
		stale = append(stale, path.Join(o.Label, rel))
	}
	if p.dropRecord {
		stale = append(stale, path.Join(o.Label, ManifestName))
	}

	modified = append(append(modified, p.edited...), p.foreign...)
	sort.Strings(stale)
	sort.Strings(modified)
	return stale, modified, nil
}

// plan is what writing one tree would do to the files already in it.
type plan struct {
	// remove are unproduced recorded files whose bytes still prove loc wrote them, relative to Dir.
	remove []string
	// dropRecord is set when the tree has a record and this build writes none.
	dropRecord bool
	// edited are unproduced recorded files changed after loc wrote them, labelled.
	edited []string
	// foreign are unrecorded files with different bytes that this build would overwrite, labelled.
	foreign []string
}

// plan compares the previous record, the staged files and the disk, without writing.
func (o *Output) plan() (plan, error) {
	var p plan

	prev, err := o.previous()
	if err != nil || prev == nil {
		// With no record nothing here is provably loc's, so nothing is removed.
		return p, err
	}
	realDir, err := resolve(o.Dir)
	if err != nil {
		return p, err
	}

	want := make(map[string]struct{}, len(o.files))
	for _, f := range o.Files() {
		rel := path.Clean(f.Path)
		want[rel] = struct{}{}
		if _, recorded := prev[rel]; recorded {
			continue
		}
		// An unrecorded path is foreign unless it already holds the bytes this build writes.
		body, err := os.ReadFile(filepath.Join(o.Dir, filepath.FromSlash(rel)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return p, fmt.Errorf("generate: read %s: %w", filepath.Join(o.Dir, filepath.FromSlash(rel)), err)
		}
		if !bytes.Equal(body, f.Bytes) {
			p.foreign = append(p.foreign, path.Join(o.Label, rel))
		}
	}

	for rel, sum := range prev {
		if _, ok := want[rel]; ok {
			continue
		}
		if rel == ManifestName {
			// The record cannot prove itself, so it is removed by name, last.
			p.dropRecord = true
			continue
		}
		state, err := o.prove(realDir, rel, sum)
		if err != nil {
			return p, err
		}
		switch state {
		case foundRecorded:
			p.remove = append(p.remove, rel)
		case foundOther:
			p.edited = append(p.edited, path.Join(o.Label, rel))
		}
	}

	sort.Strings(p.remove)
	sort.Strings(p.edited)
	sort.Strings(p.foreign)
	return p, nil
}

// refusal stops a build before it overwrites or removes a file loc cannot prove it wrote.
func refusal(edited, foreign []string) error {
	var why []string
	switch len(edited) {
	case 0:
	case 1:
		why = append(why, edited[0]+" was modified since loc wrote it")
	default:
		why = append(why, strings.Join(edited, ", ")+" were modified since loc wrote them")
	}
	switch len(foreign) {
	case 0:
	case 1:
		why = append(why, foreign[0]+" is not in loc's record, so loc will not overwrite it")
	default:
		why = append(why, strings.Join(foreign, ", ")+" are not in loc's record, so loc will not overwrite them")
	}
	if len(why) == 0 {
		return nil
	}
	them := "it"
	if len(edited)+len(foreign) > 1 {
		them = "them"
	}
	return fmt.Errorf("generate: %s; remove %s by hand", strings.Join(why, "; "), them)
}

// found is what a recorded path holds now.
type found int

const (
	foundNothing found = iota
	foundRecorded
	foundOther
)

// prove reports whether a recorded path still holds loc's bytes; outside the tree is an error.
func (o *Output) prove(realDir, rel, sum string) (found, error) {
	full := filepath.Join(o.Dir, filepath.FromSlash(rel))

	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		// Already gone; nothing to report.
		return foundNothing, nil
	}
	if err != nil {
		return foundNothing, fmt.Errorf("generate: stat %s: %w", full, err)
	}
	// loc writes regular files only; a symlink or directory there is not loc's.
	if !info.Mode().IsRegular() {
		return foundOther, nil
	}

	// inside checked the text; a symlinked directory on the way can still lead outside.
	real, err := resolve(full)
	if err != nil {
		return foundNothing, err
	}
	if !config.Under(realDir, real) {
		return foundNothing, fmt.Errorf("generate: %s names %q, which resolves to %s, outside %s",
			filepath.Join(o.Dir, ManifestName), rel, real, o.Dir)
	}

	body, err := os.ReadFile(full)
	if err != nil {
		return foundNothing, fmt.Errorf("generate: read %s: %w", full, err)
	}
	if hashOf(body) != sum {
		return foundOther, nil
	}
	return foundRecorded, nil
}

// previous maps each recorded file to the sha256 loc wrote, or is nil without a record.
func (o *Output) previous() (map[string]string, error) {
	name := filepath.Join(o.Dir, ManifestName)

	raw, err := os.ReadFile(name)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("generate: read %s: %w", name, err)
	}

	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		return nil, fmt.Errorf("generate: %s: %w", name, err)
	}
	if man.SchemaVersion != ManifestSchemaVersion {
		return nil, fmt.Errorf("generate: %s: schemaVersion %d, want %d; remove that record by hand, and any file it lists that loc wrote",
			name, man.SchemaVersion, ManifestSchemaVersion)
	}

	// A record can be hand-edited, so an entry outside the directory or without a hash is an error.
	prev := make(map[string]string, len(man.Files)+1)
	for rel, sum := range man.Files {
		clean := path.Clean(rel)
		switch {
		case !inside(rel):
			return nil, fmt.Errorf("generate: %s names %q, which is outside %s", name, rel, o.Dir)
		case clean == ".":
			return nil, fmt.Errorf("generate: %s names %q, which is the directory itself", name, rel)
		case !isSum(sum):
			return nil, fmt.Errorf("generate: %s gives %q the hash %q, which is not a sha256; remove that record by hand, and any file it lists that loc wrote",
				name, rel, sum)
		}
		if _, dup := prev[clean]; dup {
			return nil, fmt.Errorf("generate: %s names %q twice", name, clean)
		}
		prev[clean] = sum
	}
	// The record itself, removed by name rather than by hash.
	prev[ManifestName] = ""
	return prev, nil
}

// isSum reports whether s is a sha256 as hashOf spells it: 64 lowercase hex digits.
func isSum(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// inside reports whether a recorded path stays within the directory that recorded it.
func inside(rel string) bool {
	if filepath.IsAbs(rel) || path.IsAbs(rel) {
		return false
	}
	clean := path.Clean(rel)
	return clean != ".." && !strings.HasPrefix(clean, "../")
}

// hashOf is the one content hash: the record writes it and the sweep checks it.
func hashOf(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// resolve is path made absolute with every symlink in it followed.
func resolve(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("generate: resolve %s: %w", p, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("generate: resolve %s: %w", p, err)
	}
	return real, nil
}

// pruneEmptied removes directories a removal left empty, deepest first, never root itself.
func pruneEmptied(root string, dirs []string) error {
	// Only directories that lost a file are examined; other empty directories stay.
	seen := map[string]struct{}{}
	var ordered []string

	for _, dir := range dirs {
		for cur := dir; cur != root && config.Under(root, cur); cur = filepath.Dir(cur) {
			if _, dup := seen[cur]; dup {
				break
			}
			seen[cur] = struct{}{}
			ordered = append(ordered, cur)
		}
	}
	if len(ordered) == 0 {
		return nil
	}
	sort.Sort(sort.Reverse(sort.StringSlice(ordered)))

	realRoot, err := resolve(root)
	if err != nil {
		return err
	}
	for _, dir := range ordered {
		// Skip a directory reached through a symlink; removing it would follow the link.
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			continue
		}
		real, err := resolve(dir)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if real != filepath.Join(realRoot, rel) {
			continue
		}
		if err := removeIfEmpty(dir); err != nil {
			return err
		}
	}
	return nil
}

// removeIfEmpty removes dir when it is an empty directory; a symlink or missing path is left alone.
func removeIfEmpty(dir string) error {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("generate: stat %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("generate: read %s: %w", dir, err)
	}
	if len(entries) > 0 {
		return nil
	}
	if err := os.Remove(dir); err != nil {
		return fmt.Errorf("generate: remove %s: %w", dir, err)
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
