// internal/config/config.cue

package config

#Config: {
	// The locale everything falls back to. Must appear in locales.
	defaultLocale: string

	locales: [...string] & [_, ...]
	targets: [...#Target] & [_, ...]

	// Canonical roots, in order. A later root overrides an earlier one.
	roots: [...string] & [_, ...]

	// Generated artefacts, committed. Relative to this file.
	output: string | *"generated"

	// Where one target's files go, overriding "<output>/<target>". Relative to this file.
	// ".." is allowed so a Go bundle can reach the package that embeds it.
	outputs: [#Target]: string

	// Namespace ownership. A namespace with no entry, or no targets, reaches defaultTargets.
	namespaces: [string]: #Namespace

	// What a namespace without its own targets reaches. Empty means every target.
	defaultTargets: [...#Target]

	// Keys built at runtime, kept out of the unused report.
	// A trailing dot covers everything beneath it: "status." covers status.pending.
	dynamicKeys: [...string]

	// Directories to scan for key references. Empty disables the unused and missing reports.
	scan: [...string]

	// Extra Go regexps, each with one capture group holding the key.
	// Built-ins cover t(), $t(), apierr.New/Wrap and the generated Go accessors.
	keyPatterns: [...string]

	go: #Go @go(Go)
}

#Target: "backend" | "web"

#Namespace: {
	targets: [...#Target]
}

#Go: {
	// Package name for generated Go. Empty disables Go codegen.
	package: string | *"messages"
}
