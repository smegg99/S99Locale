# CLI

```
loc build      validate and generate every configured target
loc check      validate and confirm the output is current. Never writes
loc stats      counts by namespace, target and locale, plus bundle sizes
loc lint       report messages that may be duplicates
loc watch      rebuild on every change until interrupted
loc refactor-shared   plan a rename, as a patch
loc clean      remove generated artefacts
```

Flags:

```
-c, --config path   config file (default i18n/config.cue)
    --strict        warnings fail the build
    --quiet         suppress success output; diagnostics are unaffected
-v, --version       print the version and exit
```

`check` and the `lint` subcommands also take `--json`.

Errors always fail. Warnings fail only under `--strict`.

## Install

```sh
go install github.com/smegg99/s99locale/cmd/loc@v0.1.0
```

Pin the version. The binary is `cmd/loc`, so `go install` names it `loc`.

`loc --version` reports what is running - `0.1.0` from a tagged install,
`0.1.0 (a22e8d3, dirty)` from a working tree - so a CI log says which binary
produced a result.

Runs touching one output directory serialise on an advisory lock, so `loc watch`
and a manual `loc build` cannot interleave. `check` takes a shared lock, so
concurrent checks do not queue behind each other.

## Exit codes

| code | meaning |
|---|---|
| 0 | fine |
| 1 | the locales are wrong, or the output is stale. Diagnostics were printed |
| 2 | loc could not run: no config, unreadable path, unknown command |

CI should treat 1 as a failing check and 2 as a broken job. `check --json`
uses the same codes, so a script can key on the exit status and read the body
only when it needs detail.

```json
{
  "current": false,
  "files": 0,
  "stale": [],
  "errors": 1,
  "warnings": 0,
  "diagnostics": [
    {
      "code": "I18N005",
      "severity": "error",
      "key": "common.cancel",
      "locale": "pl",
      "source": {"file": "i18n/locales/en/common.yaml", "line": 2, "col": 1},
      "detail": "missing from pl; it exists in en"
    }
  ]
}
```

## Config reference

```cue
defaultLocale: "en"              // must appear in locales
locales: ["en", "pl"]            // each needs a plural rule
targets: ["web", "backend"]      // web | backend | desktop

roots: [                         // later roots override earlier ones
	"../S99AB/i18n/locales",
	"locales",
]
output: "generated"              // relative to the config file

namespaces: {
	common: {targets: ["web"]}
	emails: {targets: ["backend"]}
}

dynamicKeys: ["status.pending"]  // built at runtime, so no scan can find them

web: {
	pluralsFile: "plurals.gen.ts"
	keysFile:    "keys.gen.ts"
}
go: {package: "messages"}        // empty disables Go codegen
```

Every relative path resolves against the config file's directory, so `loc` can
run from anywhere.

A namespace with no entry reaches every target. Shipping a spare string costs
bytes; dropping one silently costs a translation.

## Layered roots

A library ships the keys its own components call; an app supplies the rest and
may override any of them:

```cue
roots: ["../S99AB/i18n/locales", "locales"]
```

Every override is reported (I18N016).

## Build order

```
loc build  ->  loc check  ->  application build
```

The Go side embeds its bundle, so generation has to happen first. A justfile
dependency enforces the order:

```just
build: locales
    go build -trimpath -o bin/app .

locales:
    loc build -c i18n/config.cue

check-locales:
    loc check -c i18n/config.cue
```

## CI

Run `loc check` before the tests. It fails when a translation is missing, a
placeholder disagrees, a plural band is absent, or the generated output on disk
is stale.

`check` compares against the files that are there. A missing file counts as
stale, so on a clean checkout with `generated/` gitignored it fails until
something has run `loc build`:

```just
ci: locales check-locales test

locales:
    loc build -c i18n/config.cue

check-locales:
    loc check -c i18n/config.cue
```

Building first and then checking is what proves the build is deterministic: if
`build` were not, `check` would find its own output stale. Commit `generated/`
instead and `check` alone is enough.
