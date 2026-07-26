// The example is a module of its own so its generated accessors, which import
// go-i18n, stay outside `go vet ./...` at the repo root.
//
// Its output is committed like every other generated tree, and
// `just check-example` points the compiler at it.

module example.invalid/basic

go 1.27

require github.com/nicksnyder/go-i18n/v2 v2.6.1

require golang.org/x/text v0.32.0 // indirect
