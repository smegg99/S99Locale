// examples/basic/i18n/config.cue

defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]

roots: ["locales"]

// The Go accessors go into the example module itself, so the compiler checks them.
outputs: {backend: "../messages"}

namespaces: {
	// Shown by Vue, never rendered by Go.
	app: {targets: ["web"]}
	// Rendered by Go into an email, never shown by Vue.
	emails: {targets: ["backend"]}
}

go: {package: "messages"}
