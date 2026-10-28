# S99Locale

One canonical locale source. Compiles to per-target bundles.

```
loc build      generate every configured target
loc check      validate and confirm the output is current. CI
loc stats      counts by namespace, target and locale
loc lint       unused, missing and duplicate keys
loc watch      rebuild on every change until interrupted
loc clean      remove what the build record names
```

Exit 1 means the locales are wrong or the output is stale; exit 2 means `loc`
could not run at all. `check` and `lint` take `--json` for CI. `--lang` and
`--color` are persistent.

## Prerequisites

- Go 1.27. `go.mod` names it and there is no toolchain pin.
- `just`, for the recipes below. Every one of them is a plain command you can
  also run by hand.
- Node 24 or newer, for `just check` only. The CLDR plural test runs the
  generated index against `Intl.PluralRules`, and `check-node` fails rather
  than letting that test skip.

Nothing else. `cue` is not needed: the schema is embedded in the binary.

## The problem

go-i18n wants flat keys and `{{.Name}}`. vue-i18n wants nested keys, `{name}`,
and plural bands separated by pipes. `loc` writes both from one canonical
source and checks that every key exists in every locale, that placeholders
agree, and that each plural has the forms its locale needs.

## Install

In a repository that uses loc, pin it as a tool:

```sh
go get -tool github.com/smegg99/s99locale/cmd/loc@v1.1.0
go tool loc build -c i18n/config.cue
```

There is no PATH install. The version lives in the consuming repository's own
`go.mod`, so every machine that builds it runs the same `loc`. For a one-off
run in a repository that does not pin it:

```sh
go run github.com/smegg99/s99locale/cmd/loc@v1.1.0 build -c i18n/config.cue
```

`loc --version` prints `loc 1.1.0` from a pinned tool directive and
`loc 1.1.0 (a22e8d3, dirty)` from a working tree.

`go tool loc` needs a warm module cache or a network. A build host that reads
committed generated output needs neither.

## Messages

```yaml
loginFailed:
  value: "Could not sign in as {email}"
  description: "Shown after an unsuccessful login attempt"
  placeholders:
    email: {type: string}
```

Plurals name their counted argument, because real sources use both `count` and
`n`:

```yaml
reeds:
  plural:
    argument: count
    forms:
      one:  "{count} głos"
      few:  "{count} głosy"
      many: "{count} głosów"
      other: "{count} głosów"
  placeholders:
    count: {type: integer}
```

Canonical files use `{name}` and nothing else.

## Plurals

CLDR Polish has four categories: one, few, many, other. English has two.
Declaring a category a locale does not have is an error: a `few` in English is
text no rule will ever select.

Which band a count lands in comes from `Intl.PluralRules` in the browser and
from go-i18n on the backend. Both are CLDR, so the two sides cannot disagree.

Each runtime gets what it needs:

| runtime | form |
|---|---|
| vue-i18n | `{count} głos \| {count} głosy \| {count} głosów \| {count} głosów` |
| go-i18n | `{one, few, many, other}`, its own native categories |

The pipe order is a contract between the generated strings and the generated
`plurals.gen.ts`. Both are generated, so they stay in step.

## Config

```cue
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]

// Later roots override earlier ones. Every override is reported.
roots: ["../S99AB/i18n/locales", "locales"]
// Generated artefacts, committed. Relative to this file.
output: "generated"

// Where one target's files go, overriding "<output>/<target>". This is how a
// Go bundle reaches the package that embeds it, since go:embed cannot leave
// its own directory.
outputs: {backend: "../backend/common/messages"}

namespaces: {
	common: {targets: ["web"]}
	emails: {targets: ["backend"]}
}

go: {package: "messages"}
```

A namespace with no entry reaches every target, or the ones `defaultTargets` names (for
example `defaultTargets: ["web"]` when most namespaces are frontend-only).

## Layered roots

A library can ship the keys its own components call, and an app can override
any of them:

```
roots: ["../S99AB/i18n/locales", "locales"]
```

`loc` reports each override as warning I18N016.

## Output

```
generated/backend/messages_gen.go      typed accessors
generated/backend/keys_gen.go          Key* consts, DefaultLocale, Locales
generated/backend/embed_gen.go         LocaleFiles (go:embed) and NewBundle()
generated/backend/locales/{en,pl}.json flat, {{.Name}}, go-i18n
generated/backend/loc-manifest.json    the build record
generated/web/{en,pl}/*.json           nested, {name}, one file per namespace
generated/web/plurals.gen.ts           the index function
generated/web/keys.gen.ts              the locale list and the default locale
generated/web/loc-manifest.json        the build record
```

One directory per target, with the same shape inside it whether it sits under
`output` or is redirected by `outputs`. The config above puts the backend tree
in `../backend/common/messages` instead, and the backend files land there.

`NewBundle()` returns a go-i18n bundle with every locale loaded, so a package
needs no loader of its own:

```go
bundle := messages.NewBundle()
localizer := i18n.NewLocalizer(bundle, "pl", messages.DefaultLocale)
```

Deterministic: sorted keys, no timestamps, atomic writes, unchanged files left
alone. Two builds on the same input produce identical bytes, and the generated
Go is already gofmt-canonical so a repo-wide `gofmt -w .` cannot make it stale.

Generated output is committed. `loc check` on its own then catches output that
was not regenerated, and a fresh clone builds without running `loc` first.

`loc-manifest.json` in each target directory is the build record. It is what
the next build's removal sweep reads: loc removes exactly the files that record
names, that this build does not produce, and whose bytes still hash to what the
record says. A file loc cannot prove it wrote is never removed or overwritten:
the build refuses before it touches anything, names the file, and asks for it to
be removed by hand.

`loc clean` cannot remove a tree it has no record of, such as one whose
`loc-manifest.json` was deleted. Remove that tree by hand.

## Build order

```
loc build  ->  loc check  ->  application build
```

The Go side embeds its bundle, so `loc build` has to run before `go build`. A
justfile dependency enforces the order:

```just
build: locales
    go build -trimpath -o bin/app .

locales:
    go tool loc build -c i18n/config.cue

check-locales:
    go tool loc check -c i18n/config.cue
```

## Adding a message

1. Add it to `i18n/locales/en/<namespace>.yaml`.
2. Add the same key to every other locale. `loc check` fails until you do.
3. Run `just locales`.
4. Use `t('namespace.key')` in Vue, or the generated accessor in Go.

## Adding a locale

1. Add it to `locales` in `config.cue`. A locale with no CLDR plural rule is
   rejected at load.
2. Create `i18n/locales/<code>/`.
3. `loc stats` prints how many translations are still missing.

## Diagnostics

```
[x] failed   placeholder mismatch  code=I18N003
 Key:     auth.login.failed
 en:      {email}
 pl:      {emailAddress}
 Source:  i18n/locales/pl/auth/login.yaml:14:5
```

Errors fail the build. Warnings do not, unless `--strict`. The glyph and the
word both carry the state, so the line reads the same with colour stripped.

| code | meaning |
|---|---|
| I18N001 | invalid YAML |
| I18N002 | message does not fit the schema |
| I18N003 | placeholder mismatch across locales |
| I18N004 | duplicate fully qualified key |
| I18N005 | key missing from a locale |
| I18N007 | empty translation |
| I18N008 | message kind differs across locales |
| I18N009 | required plural category missing |
| I18N010 | plural category the locale does not have |
| I18N011 | plural argument differs across locales |
| I18N012 | placeholder used but not declared |
| I18N013 | placeholder declared but never used (warning) |
| I18N014 | placeholder type differs across locales |
| I18N015 | two placeholders collide as one Go field |
| I18N016 | a later root overrode an earlier one (warning) |
| I18N018 | plural argument is not an integer |
| I18N019 | `{{` or `}}` in the text, which go-i18n reads as a template |

`lint` validates the locales before it looks for anything of its own, so a
broken source fails it. Its own findings are reported under these codes and
fail only under `--strict`, and only for `lint keys`:

| code | meaning | command |
|---|---|---|
| I18N021 | possible shared message | `lint shared` |
| I18N030 | referenced but not declared | `lint keys` |
| I18N032 | declared but not referenced | `lint keys` |

## API errors

Backend error responses carry a stable code, not a localised string:

```json
{"code": "AUTH_INVALID_CREDENTIALS", "params": {"remainingAttempts": 2}}
```

The frontend maps the code to a key, so the API contract does not depend on
translation key names.

## Documentation

- [docs/format.md](docs/format.md): canonical file format
- [docs/plurals.md](docs/plurals.md): categories per locale, and the numbers
- [docs/cli.md](docs/cli.md): commands, config reference, build integration
- [docs/sharing.md](docs/sharing.md): duplicate detection and confidence
- [docs/scanning.md](docs/scanning.md): finding unused and missing keys
- [examples/basic](examples/basic): en + pl, both targets, end to end

## Shared messages

```sh
loc lint shared      # keys that may be one message
loc lint duplicates  # exact duplicate contracts
```

High confidence needs the whole contract to match in every locale, not just
the default one: `file.open` and `store.status.open` are both "Open" in English
and different words in Polish. See [docs/sharing.md](docs/sharing.md).

## Watching

```sh
loc watch
```

Rebuilds on every save, debounced. A validation error prints and the watcher
keeps going.

## Scanning

```sh
loc lint keys   # referenced but not declared, declared but not referenced
```

See [docs/scanning.md](docs/scanning.md).

## loc's own text

loc is localized with itself. `i18n/config.cue` builds its catalogue into
`internal/catalog`, which the binary embeds, and that output is committed like
any other. `--lang pl` renders the whole command tree in Polish, including
cobra's own headings; `LOC_LANG`, `LC_ALL`, `LC_MESSAGES` and `LANG` are
consulted in that order when the flag is absent.

Two consequences:

- `loc clean -c i18n/config.cue` removes the embedded bundle, so the package
  stops compiling until the next build. The recovery is
  `git checkout -- internal/catalog`.
- A tree whose catalogue is unbuildable can be regenerated with a released
  binary, because `pkg@version` ignores the current module:
  `go run github.com/smegg99/s99locale/cmd/loc@v1.1.0 build -c i18n/config.cue`.

## Licence

MIT.
