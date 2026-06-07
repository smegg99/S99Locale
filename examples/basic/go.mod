// The example is a module of its own for two reasons: `loc build` writes Go in
// here that imports go-i18n, which loc itself does not depend on, and that
// output must stay outside `go vet ./...` at the repo root.
//
// Nothing is committed here to build. The dependency exists so a test can
// compile the generated accessors and prove they are more than well-formed.

module example.invalid/basic

go 1.26

require github.com/nicksnyder/go-i18n/v2 v2.6.1

require golang.org/x/text v0.32.0 // indirect
