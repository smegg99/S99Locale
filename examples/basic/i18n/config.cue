// examples/basic/i18n/config.cue

defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]

roots: ["locales"]
output: "generated"

namespaces: {
	// Shown by Vue, never rendered by Go.
	app: {targets: ["web"]}
	// Rendered by Go into an email, never shown by Vue.
	emails: {targets: ["backend"]}
}

go: {package: "messages"}
