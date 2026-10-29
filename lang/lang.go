// lang/lang.go

// Package lang resolves the language a command-line program renders in.
package lang

import (
	"os"
	"strings"
)

// osVars are the OS variables in POSIX precedence; the first non-empty one decides.
var osVars = [...]string{"LC_ALL", "LC_MESSAGES", "LANG"}

// Options describes one program's language selection.
type Options struct {
	// Var is the program's own variable, checked before the OS ones; empty skips it.
	Var string
	// Supported are the languages with a catalogue, in config order, such as the generated Locales.
	Supported []string
	// Default is returned when nothing selects a supported language; it must be in Supported.
	Default string
	// LookupEnv defaults to os.LookupEnv; a test supplies its own.
	LookupEnv func(string) (string, bool)
}

// Detect returns a member of Supported.
func Detect(opts Options) string {
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	names := make([]string, 0, len(osVars)+1)
	if opts.Var != "" {
		names = append(names, opts.Var)
	}
	names = append(names, osVars[:]...)

	for _, name := range names {
		value, ok := lookup(name)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		// The first variable with a value decides; an unsupported one falls back rather than failing.
		if match, ok := Match(opts.Supported, value); ok {
			return match
		}
		return opts.Default
	}
	return opts.Default
}

// Match returns the supported locale value selects: same language and region, else the bare language, else the first match.
func Match(supported []string, value string) (canonical string, ok bool) {
	language, region, isC := split(value)
	if isC || language == "" {
		return "", false
	}

	best := 0
	for _, entry := range supported {
		entryLanguage, entryRegion, _ := split(entry)
		if entryLanguage != language {
			continue
		}
		rank := 1
		switch entryRegion {
		case region:
			rank = 3
		case "":
			rank = 2
		}
		if rank > best {
			canonical, best = entry, rank
		}
	}
	return canonical, best > 0
}

// split lowercases a locale into language and region, dropping .encoding and @modifier.
func split(value string) (language, region string, isC bool) {
	tag := strings.TrimSpace(value)
	tag, _, _ = strings.Cut(tag, "@")
	tag, _, _ = strings.Cut(tag, ".")
	if upper := strings.ToUpper(tag); upper == "C" || upper == "POSIX" {
		return "", "", true
	}

	language, region, _ = strings.Cut(strings.ToLower(strings.ReplaceAll(tag, "_", "-")), "-")
	return language, region, false
}
