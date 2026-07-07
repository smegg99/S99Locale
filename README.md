# S99Locale

One canonical locale source. Compiles to per-target bundles.

```
loc build      generate every configured target
loc check      validate and confirm the output is current. CI
loc stats      counts by namespace, target and locale
loc clean      remove generated artefacts
```

Exit 1 means the locales are wrong or the output is stale; exit 2 means `loc`
could not run at all. `check` takes `--json` for CI.

## The problem

go-i18n wants flat keys and `{{.Name}}`. vue-i18n wants nested keys, `{name}`,
and plural bands separated by pipes. `loc` writes both from one canonical
source and checks that every key exists in every locale, that placeholders
agree, and that each plural has the forms its locale needs.

## Install

```sh
go install github.com/smegg99/s99locale/cmd/loc@v0.1.0   # pinned, for CI
just install                                             # local, from source
```

Pin a version in CI. `loc --version` reports what is actually running -
`0.1.0` from a tagged install, `0.1.0 (a22e8d3, dirty)` from a working tree -
so a log says which binary produced a result.

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
      zero: "brak głosów"
      one:  "{count} głos"
      few:  "{count} głosy"
      many: "{count} głosów"
      other: "{count} głosów"
  placeholders:
    count: {type: integer}
```

Optional per message: `targets`, `deprecated`, `sharing: {isolate, reason}`.

Canonical files use `{name}` and nothing else.

## Plurals

CLDR Polish has four categories: one, few, many, other. English has two.
Declaring a category a locale does not have is an error: a `few` in English is
text no rule will ever select.

`zero` is an **opt-in extra band, not a CLDR category**. No rule returns it:
Polish resolves a count of 0 to `many`, so `0 głosów` is correct but `brak
głosów` usually reads better. If one locale of a message declares `zero`, all
must - that is I18N019, and it is the one part of a plural's shape that has to
agree, since the required categories differ by locale on purpose.

Which band a count lands in comes from `Intl.PluralRules` in the browser and
from go-i18n on the backend. Both are CLDR, so the two sides cannot disagree.

Each runtime gets what it needs:

| runtime | form |
|---|---|
| vue-i18n | `brak głosów \| {count} głos \| {count} głosy \| {count} głosów` |
| go-i18n | `{one, few, many, other}` plus a separate `key.zero` message |

The pipe order is a contract between the generated strings and the generated
`plurals.gen.ts`. Both are generated, so they stay in step.

In Go the `zero` band is reachable only through the generated accessor:

```go
func ReedsFound(l *i18n.Localizer, p ReedsFoundParams) string {
	// No CLDR rule selects "zero"; it is an opt-in band.
	if p.Count == 0 {
		return zeroForm(l, "tuner.reeds", data)
	}
	return withCount(l, "tuner.reeds", p.Count, data)
}
```

## Config

```cue
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web", "backend"]

// Later roots override earlier ones. Every override is reported.
roots: ["../S99AB/i18n/locales", "locales"]
output: "generated"

namespaces: {
	common: {targets: ["web"]}
	emails: {targets: ["backend"]}
}

go: {package: "messages"}
```

A namespace with no entry reaches every target.

## Layered roots

A library can ship the keys its own components call, and an app can override
any of them:

```
roots: ["../S99AB/i18n/locales", "locales"]
```

`loc` reports each override as warning I18N016.

## Output

```
generated/
├── backend/{en,pl}.json      flat, {{.Name}}, go-i18n
├── web/{en,pl}/*.json        nested, {name}, one file per namespace
├── web/plurals.gen.ts        the index function
├── web/keys.gen.ts           the locale list
├── go/{messages,keys}_gen.go typed accessors
└── manifest.json
```

Deterministic: sorted keys, no timestamps, atomic writes, unchanged files left
alone. Two builds on the same input produce identical bytes, and the generated
Go is already gofmt-canonical so a repo-wide `gofmt -w .` cannot make it stale.

Generated output is disposable. Gitignore it, and have CI build before it
checks - `check` counts a missing file as stale, so it cannot stand alone in a
clean checkout.

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
    loc build -c i18n/config.cue

check-locales:
    loc check -c i18n/config.cue
```

Committing `generated/` instead flips it: then `loc check` on its own catches
an output nobody regenerated.

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
I18N003 placeholder mismatch

  Key:     auth.login.failed
  en:      {email}
  pl:      {emailAddress}
  Source:  i18n/locales/pl/auth/login.yaml:14:5
```

Errors fail the build. Warnings do not, unless `--strict`.

| code | meaning |
|---|---|
| I18N001 | invalid YAML |
| I18N002 | message does not fit the schema |
| I18N003 | placeholder mismatch across locales |
| I18N004 | duplicate fully qualified key |
| I18N005 | key missing from a locale |
| I18N006 | unknown target |
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
| I18N017 | `sharing.isolate` with no reason, under `--strict` |
| I18N018 | plural argument is not an integer |
| I18N019 | the `zero` band is declared in some locales but not others |

`lint` reports under its own codes and never fails a build:

| code | meaning | command |
|---|---|---|
| I18N021 | possible shared message | `lint shared` |
| I18N030 | referenced but not declared | `lint keys` |
| I18N031 | deprecated key still referenced | `lint keys` |
| I18N032 | declared but not referenced | `lint keys` |

## API errors

Backend error responses carry a stable code, not a localised string:

```json
{"code": "AUTH_INVALID_CREDENTIALS", "params": {"remainingAttempts": 2}}
```

The frontend maps the code to a key, so the API contract does not depend on
translation key names.

## Documentation

- [docs/format.md](docs/format.md) - canonical file format
- [docs/plurals.md](docs/plurals.md) - categories, the zero band, the numbers
- [docs/cli.md](docs/cli.md) - commands, config reference, build integration
- [docs/sharing.md](docs/sharing.md) - duplicate detection and confidence
- [docs/pseudolocale.md](docs/pseudolocale.md) - what it catches and preserves
- [docs/scanning.md](docs/scanning.md) - finding unused and missing keys
- [docs/refactoring.md](docs/refactoring.md) - planning a rename
- [examples/basic](examples/basic) - en + pl, both targets, end to end

## Importing an existing project

```sh
loc import --from-go common/i18n/locales --from-vue frontend/i18n/locales
```

Reads flat go-i18n bundles and nested vue-i18n bundles, writes canonical YAML
into the last configured root, and reports everything it could not work out -
ambiguous plural bands, a counted argument it had to guess, the two trees
disagreeing about one string. Descriptions are never recoverable from a bundle
and are always worth adding afterwards.

Existing canonical files are left alone unless `--force`, so an import cannot
silently drop work someone has already done.

`loc build` emits exactly the two formats `loc import` reads, which makes
build -> import -> build a round trip that has to land on identical bytes. That
is the importer's real test.

## Shared messages and pseudolocalisation

```sh
loc lint shared      # keys that may be one message
loc lint duplicates  # exact duplicate contracts
```

High confidence needs the whole contract to match in every locale, not just
the default one: `file.open` and `store.status.open` are both "Open" in English
and different words in Polish. See [docs/sharing.md](docs/sharing.md).

```cue
pseudolocale: true
```

`Sign in` becomes `[Šïğñ ïñ··]` - accented so untranslated text stands out,
padded so a tight layout overflows, bracketed so truncation is visible, with
placeholders untouched. See [docs/pseudolocale.md](docs/pseudolocale.md).

## Watching

```sh
loc watch
```

Rebuilds on every save, debounced. A validation error prints and the watcher
keeps going.

## Scanning and refactoring

```sh
loc lint keys                                    # unused, missing, deprecated
loc refactor-shared a.b c.d --to common.action.x # prints a patch, writes nothing
```

See [docs/scanning.md](docs/scanning.md) and
[docs/refactoring.md](docs/refactoring.md).

## Licence

MIT.
