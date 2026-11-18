# CLI

```
loc build             validate and generate every configured target
loc check             validate and confirm the output is current. Never writes
loc stats             counts by namespace, target and locale, plus bundle sizes
loc lint shared       keys that may belong in a shared namespace
loc lint keys         unused and missing keys, from a source scan
loc lint duplicates   exact duplicate contracts, whatever they are named
loc watch             rebuild on every change until interrupted
loc clean             remove what the build record names
```

Persistent flags:

```
-c, --config path        config file (default i18n/config.cue)
    --quiet              suppress success output; diagnostics are unaffected
    --lang code          render in this language
    --color auto|always|never
-v, --version            print the version and exit
```

Per-command flags:

```
build --strict           warnings fail the build
check --strict --json
watch --strict
lint --strict --json     inherited by shared, keys and duplicates
lint shared --verbose    include medium and low confidence findings
```

`--strict` is on the four commands that act on it and nowhere else. Errors
always fail. Warnings fail only under `--strict`, and `lint keys --strict`
fails on an unused or missing key.

Language precedence: `--lang`, then `LOC_LANG`, `LC_ALL`, `LC_MESSAGES`,
`LANG`, then English. An unsupported `--lang` is a usage error and exits 2; an
unsupported `LOC_LANG` falls back to English.

## Install

```sh
go get -tool github.com/smegg99/s99locale/cmd/loc@v1.1.2
go tool loc build -c i18n/config.cue
```

The version is pinned in the consuming repository's own `go.mod`, so every
machine that builds it runs the same `loc`.

`loc --version` prints `loc 1.1.2` from a pinned tool directive and
`loc 1.1.2 (a22e8d3, dirty)` from a working tree.

Runs serialise on one advisory lock per target directory, so `loc watch` and a
manual `loc build` cannot interleave. `check` takes shared locks, so concurrent
checks do not queue behind each other. A run that has to wait says so, and
ctrl-c ends the wait.

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
  "modified": [],
  "errors": 1,
  "warnings": 0,
  "diagnostics": [
    {
      "code": "I18N005",
      "severity": "error",
      "message": "missingKey",
      "args": {"base": "en", "locale": "pl"},
      "key": "common.cancel",
      "locale": "pl",
      "source": {"file": "i18n/locales/en/common.yaml", "line": 2, "col": 1}
    }
  ]
}
```

`message` is the catalogue key for the sentence and `args` are its
placeholders, because one code is raised at several sites that say different
things. A diagnostic that wraps a third-party parser's own text carries it in
`cause`, untranslated. `stale` is what `loc build` would rewrite; `modified` is
what it refuses to touch.

## Config reference

```cue
defaultLocale: "en"              // must appear in locales
locales: ["en", "pl"]            // each needs a plural rule
targets: ["web", "backend"]      // web | backend

roots: [                         // later roots override earlier ones
	"../S99AB/i18n/locales",
	"locales",
]
output: "generated"              // relative to the config file
outputs: {backend: "../backend/common/messages"}  // one directory per target

namespaces: {
	common: {targets: ["web"]}
	emails: {targets: ["backend"]}
}

dynamicKeys: ["status.pending"]  // built at runtime, so no scan can find them

go: {package: "messages"}        // empty disables Go codegen
```

Every relative path resolves against the config file's directory, so `loc` can
run from anywhere.

A target with no `outputs` entry is written to `<output>/<target>`. Two targets
may not resolve to the same directory or to one inside the other, no target
directory may hold a canonical root or a scan directory, a redirected target
may not sit inside the `output` tree, and `output` itself must resolve below
the config's own directory. Each is refused at load.

A namespace with no entry reaches every target, or the ones `defaultTargets` names (for
example `defaultTargets: ["web"]` when most namespaces are frontend-only).

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
    go tool loc build -c i18n/config.cue

check-locales:
    go tool loc check -c i18n/config.cue
```

## CI

Run `loc check` before the tests. It fails when a translation is missing, a
placeholder disagrees, a plural band is absent, or the generated output on disk
is stale.

Generated output is committed, so `check` stands alone:

```just
check: check-locales test

check-locales:
    go tool loc check -c i18n/config.cue
    go tool loc lint keys --strict -c i18n/config.cue
```

Running `build` then `check` also shows the build is deterministic. Keep
`build` in a separate `locales` recipe, run after editing the canonical source.
