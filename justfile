app_name := "loc"
node_floor := "24"

# List available commands.
default:
    @just --list

# Run every check.
check: check-fmt vet check-node check-locales check-doc-comments test race build check-example

# Fail when a Go file is not gofmt clean.
check-fmt:
    #!/usr/bin/env bash
    set -euo pipefail
    unformatted="$(gofmt -l .)"
    if [ -n "$unformatted" ]; then
        echo "not gofmt clean:" >&2
        echo "$unformatted" >&2
        exit 1
    fi

# Fail when a doc comment carries a paragraph break.
check-doc-comments:
    ./scripts/doc-comments.sh

# Rewrite every Go file into gofmt form.
fmt:
    gofmt -w .

# Run go vet over both modules.
vet:
    go vet ./...
    cd examples/basic && go vet ./...

# Fail when node is absent or too old; the CLDR plural test skips without it.
check-node:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! command -v node >/dev/null; then
        echo "node is not on PATH; the CLDR plural test would skip" >&2
        exit 1
    fi
    major="$(node --version | sed 's/^v//' | cut -d. -f1)"
    if [ "$major" -lt {{node_floor}} ]; then
        echo "node $major is below the {{node_floor}} floor; the CLDR plural test would skip" >&2
        exit 1
    fi

# Validate the locale sources and confirm the committed output is current.
check-locales:
    go run ./cmd/loc check -c i18n/config.cue
    go run ./cmd/loc lint keys --strict -c i18n/config.cue
    go run ./cmd/loc check -c examples/basic/i18n/config.cue

# Rebuild loc's own catalogue and the example's.
locales:
    go run ./cmd/loc build -c i18n/config.cue
    go run ./cmd/loc build -c examples/basic/i18n/config.cue

# Remove every generated tree loc has a record of.
clean:
    go run ./cmd/loc clean -c i18n/config.cue
    go run ./cmd/loc clean -c examples/basic/i18n/config.cue

# Run all tests.
test:
    go test ./...

# Run the tests under the race detector.
race:
    go test -race ./...

# Build the binary into bin/.
build:
    mkdir -p bin
    go build -trimpath -o bin/{{app_name}} ./cmd/loc

# Compile the example module, which is where the generated accessors live.
check-example:
    cd examples/basic && go build ./...
