// internal/config/config.cue

package config

#Config: {
	// The locale everything falls back to. Must appear in locales.
	defaultLocale: string

	locales: [...string] & [_, ...]
	targets: [...#Target] & [_, ...]

	// Canonical roots, in order. A later root overrides an earlier one, which
	// is how an app overrides a library's string.
	roots: [...string] & [_, ...]

	// Generated artefacts. Disposable and gitignored. Relative to this file, so
	// the default lands beside it rather than in a second i18n/ beneath it.
	output: string | *"generated"

	// Namespace ownership. A namespace with no entry reaches every target: a
	// spare string costs bytes, a dropped one costs a translation.
	namespaces: [string]: #Namespace

	// Keys built at runtime, which no source scan can find. Declaring them
	// keeps them out of the unused report. A trailing dot covers everything
	// beneath it: "status." covers status.pending and the rest.
	dynamicKeys: [...string]

	// Directories to scan for key references. Empty disables scanning, and
	// then unused and missing keys are simply not reported - better than a
	// report built from nothing.
	scan: [...string]

	// Extra reference patterns, for a repo whose own helpers name keys. Each
	// is a Go regexp with exactly one capture group holding the key.
	//
	// The built-in patterns cover t(), $t(), apierr.New/Wrap and the generated
	// Go accessors. A wrapper around any of those is invisible without this.
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

	// Where the Go target is written. Empty leaves it under `output` with every
	// other target.
	//
	// go:embed cannot reach outside its own package, so a Go consumer either
	// copies the bundle into the package that embeds it or points this at that
	// package. Relative to this file, and it may sit outside `output`.
	//
	// loc owns `*_gen.go` beside your own sources there, and the whole
	// `locales/` directory inside it. Everything else in that package is yours
	// and is never read, written or removed.
	output: string | *""
}
