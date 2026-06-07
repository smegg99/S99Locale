app_name := "loc"

# List available commands.
default:
    @just --list

# Format, vet and test.
check: fmt vet test

# Format all Go files.
fmt:
    gofmt -l -w .

# Run go vet.
vet:
    go vet ./...

# Run all tests.
test:
    go test ./...

# Build the binary into bin/.
build:
    mkdir -p bin
    go build -trimpath -o bin/{{app_name}} ./cmd/loc

# Install loc onto PATH.
install: build
    sudo install -m 0755 bin/{{app_name}} /usr/local/bin/{{app_name}}

# Rebuild the example's bundles. Run from here: the example is its own module,
# and every path in a config resolves against the config file anyway.
example:
    go run ./cmd/loc build -c examples/basic/i18n/config.cue
