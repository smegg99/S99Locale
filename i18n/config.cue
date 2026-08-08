// i18n/config.cue

defaultLocale: "en"
locales: ["en", "pl"]

// loc is a CLI with no web target.
targets: ["backend"]

roots: ["locales"]

// Written into the package that embeds it, since go:embed cannot leave its own directory.
outputs: {backend: "../internal/catalog"}

// Only cmd/loc renders text; internal/ returns codes, and its scanner test fixtures
// would read as key references.
scan: ["../cmd/loc"]

go: {package: "catalog"}
