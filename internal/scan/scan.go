// internal/scan/scan.go

// Package scan finds translation keys referenced in source.
//
// Static references only: `t('status.' + state)` is unreachable from here, so
// a key with no reference is reported, never deleted.
package scan

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Reference is one place a key is named.
type Reference struct {
	Key  string
	File string
	Line int
}

// Result is everything one scan found.
type Result struct {
	// Refs maps a key to every place it appears.
	Refs map[string][]Reference
	// Files scanned, so a scan that covered nothing is visible.
	Files int
}

func (r *Result) add(key, file string, line int) {
	if r.Refs == nil {
		r.Refs = map[string][]Reference{}
	}
	r.Refs[key] = append(r.Refs[key], Reference{Key: key, File: file, Line: line})
}

// Keys returns every referenced key, sorted.
func (r *Result) Keys() []string {
	out := make([]string, 0, len(r.Refs))
	for k := range r.Refs {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// frontend matches the vue-i18n family: t, $t, te, tm, and the composable's
// own name. The key must look like a key, or every t('hello world') in a
// comment becomes a false positive.
var frontend = regexp.MustCompile(`\$?\b(?:t|te|tm)\(\s*['"` + "`" + `]([a-zA-Z][\w.]*)['"` + "`" + `]`)

// goKey matches the generated key constants, for Go naming a key it does not
// render itself.
var goKey = regexp.MustCompile(`\bKey([A-Z]\w*)\b`)

// apierrKey matches the message key in an error envelope.
//
// These are the keys a user actually reads: the envelope carries a machine
// code and an i18n key, and the frontend resolves the key into the dialog. An
// undeclared one renders as itself, which is the most visible way a missing
// translation can fail.
var apierrKey = regexp.MustCompile(`apierr\.(?:New|Wrap)\([^)]*?["` + "`" + `]([a-z][\w.]*\.[\w.]+)["` + "`" + `]`)

// goAccessor matches a call to a generated accessor.
//
// Any exported identifier followed by a paren; the GoNames map is what decides
// whether it is one of ours. Constraining the first argument instead only
// worked when the localizer happened to be called `l`, and real code calls it
// whatever the locale is.
var goAccessor = regexp.MustCompile(`\b([A-Z]\w*)\s*\(`)

// skipDirs are never worth walking and are usually enormous.
var skipDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	".nuxt":        true,
	".output":      true,
	"dist":         true,
	"vendor":       true,
	"bin":          true,
	"testdata":     true,
}

// Options configures a scan.
type Options struct {
	// Roots are directories to walk.
	Roots []string
	// Generated is skipped: the generated bundles name every key, so scanning
	// them would make every key look used and the report always empty.
	Generated string
	// GoNames maps a generated identifier back to its key, so a Go accessor
	// call resolves to something comparable.
	GoNames map[string]string
	// Extra are repo-specific patterns, each with one capture group. A repo
	// that wraps t() or apierr in its own helper is invisible without them.
	Extra []*regexp.Regexp
}

// Compile turns config patterns into matchers, rejecting one without exactly one group.
func Compile(patterns []string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, 0, len(patterns))

	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("scan: pattern %q: %w", p, err)
		}
		if re.NumSubexp() != 1 {
			return nil, fmt.Errorf(
				"scan: pattern %q has %d capture groups, want exactly one holding the key",
				p, re.NumSubexp())
		}
		out = append(out, re)
	}
	return out, nil
}

// Run walks the roots and collects references.
func Run(opts Options) (*Result, error) {
	res := &Result{Refs: map[string][]Reference{}}

	generated, _ := filepath.Abs(opts.Generated)

	for _, root := range opts.Roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				// A missing root is skipped rather than failing the scan.
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}

			if d.IsDir() {
				if skipDirs[d.Name()] {
					return fs.SkipDir
				}
				if generated != "" {
					if abs, _ := filepath.Abs(path); abs == generated {
						return fs.SkipDir
					}
				}
				return nil
			}

			if !interesting(path) {
				return nil
			}
			res.Files++
			return scanFile(res, path, opts.GoNames, opts.Extra)
		})
		if err != nil {
			return nil, fmt.Errorf("scan: walk %s: %w", root, err)
		}
	}
	return res, nil
}

func interesting(path string) bool {
	switch filepath.Ext(path) {
	case ".go", ".ts", ".js", ".vue", ".mts":
		return true
	}
	return false
}

// maxFileSize matches parse's; a larger source file is skipped, not fatal.
const maxFileSize = 4 << 20

// generatedHeader is the first line every file this tool writes carries.
//
// Skipping the output directory is not enough on its own: go:embed cannot reach
// outside its own package, so a Go consumer usually holds a copy of the
// generated bundle and accessors somewhere the scan does walk. A copy naming
// every key makes every key look used, which is exactly the report being asked
// for.
const generatedHeader = "// Code generated by loc. DO NOT EDIT."

func scanFile(res *Result, path string, goNames map[string]string, extra []*regexp.Regexp) error {
	if info, err := os.Stat(path); err == nil && info.Size() > maxFileSize {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("scan: open %s: %w", path, err)
	}
	defer file.Close()

	isGo := filepath.Ext(path) == ".go"

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64*1024), maxFileSize)

	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()

		if line == 1 && strings.HasPrefix(text, generatedHeader) {
			return nil
		}

		for _, m := range frontend.FindAllStringSubmatch(text, -1) {
			res.add(m[1], path, line)
		}
		for _, re := range extra {
			for _, m := range re.FindAllStringSubmatch(text, -1) {
				res.add(m[1], path, line)
			}
		}
		if !isGo {
			continue
		}

		for _, m := range apierrKey.FindAllStringSubmatch(text, -1) {
			res.add(m[1], path, line)
		}
		for _, m := range goKey.FindAllStringSubmatch(text, -1) {
			if key, ok := goNames[m[1]]; ok {
				res.add(key, path, line)
			}
		}
		for _, m := range goAccessor.FindAllStringSubmatch(text, -1) {
			if key, ok := goNames[m[1]]; ok {
				res.add(key, path, line)
			}
		}
	}
	return scanner.Err()
}

// Unused returns declared keys nothing references.
//
// Dynamic keys are excluded, because no scan can see them. The rest may be
// genuinely dead or a call this cannot parse, so it is only ever a warning.
func Unused(declared []string, res *Result, dynamic []string) []string {
	skip := map[string]bool{}
	for _, key := range dynamic {
		skip[key] = true
	}

	var out []string
	for _, key := range declared {
		if skip[key] || len(res.Refs[key]) > 0 {
			continue
		}
		// A declared prefix covers the keys built under it.
		if coveredByPrefix(key, dynamic) {
			continue
		}
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// Missing returns referenced keys nothing declares.
func Missing(declared []string, res *Result) []Reference {
	known := map[string]bool{}
	for _, key := range declared {
		known[key] = true
	}

	var out []Reference
	for _, key := range res.Keys() {
		if known[key] {
			continue
		}
		out = append(out, res.Refs[key][0])
	}
	return out
}

// coveredByPrefix lets `status.` in dynamicKeys cover status.pending and the rest.
func coveredByPrefix(key string, dynamic []string) bool {
	for _, d := range dynamic {
		if strings.HasSuffix(d, ".") && strings.HasPrefix(key, d) {
			return true
		}
	}
	return false
}
