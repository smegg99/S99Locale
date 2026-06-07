// version.go

// Package s99locale holds the version; the tool is cmd/loc.
package s99locale

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

// VERSION is the single source of truth. Embedding it rather than copying the
// number into a Go constant means there is no second place to update and so no
// drift to check for. It lives at the module root because go:embed cannot reach
// up a directory, which is also why this package exists.
//
//go:embed VERSION
var versionFile string

// Declared returns the version written in VERSION.
func Declared() string { return strings.TrimSpace(versionFile) }

// Version is what `loc --version` prints: the declared version, plus the commit
// it was built from when that is knowable.
//
// A module downloaded by `go install` is not a git worktree, so it carries no
// revision and reports the bare version - which is exactly right, because the
// tag it was installed at is what identifies it. A local build gets the commit
// and a dirty marker instead, so a CI log can name the binary that ran.
func Version() string {
	rev, dirty, ok := revision()
	if !ok {
		return Declared()
	}
	return format(Declared(), rev, dirty)
}

// format is split from Version because a test binary carries no VCS stamps.
func format(declared, rev string, dirty bool) string {
	if rev == "" {
		return declared
	}
	if dirty {
		return declared + " (" + rev + ", dirty)"
	}
	return declared + " (" + rev + ")"
}

func revision() (rev string, dirty, ok bool) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", false, false
	}

	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if len(rev) < 7 {
		return "", false, false
	}
	return rev[:7], dirty, true
}
