// version.go

// Package s99locale holds the version; the tool is cmd/loc.
package s99locale

import (
	_ "embed"
	"runtime/debug"
	"strings"
)

//go:embed VERSION
var versionFile string

// Declared returns the version embedded from VERSION at the module root.
func Declared() string { return strings.TrimSpace(versionFile) }

// Version is what loc --version prints: the declared version plus a local build's commit and dirty marker.
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
