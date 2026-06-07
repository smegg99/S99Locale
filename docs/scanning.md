# Source scanning

```cue
scan: ["../web/app", "../../S99AB/src", "../api", "../core"]
```

```sh
loc lint keys
```

Reports three things:

| code | meaning |
|---|---|
| I18N030 | referenced but not declared - renders the raw key |
| I18N031 | a deprecated key is still referenced |
| I18N032 | declared but not referenced |

Nothing here fails a build. Scanning is **evidence, not truth**: dynamic keys
exist, so an unused key is a warning and never a deletion.

## What it recognises

Frontend, the vue-i18n family:

```ts
t('auth.login.title')
$t("common.cancel")
te('maybe.missing')
```

Go, through the generated constants and accessors, and the error envelope:

```go
messages.ReportHeading(l)
messages.KeyReportSummary
apierr.New(apierr.NotFound, "item.notFound")
apierr.Wrap(err, apierr.Validation, "share.invalid")
```

An undeclared envelope key renders as the raw key.

A Go identifier only counts when it is one the generator emitted, so ordinary
exported functions are not mistaken for keys.

## What it cannot see

An interpolated key:

```ts
t(`status.${state}`)
```

Declare those, or every one of them shows up as unused:

```cue
dynamicKeys: ["status."]   // trailing dot covers everything beneath
```

## Your own helpers

An app that wraps `t()` or `apierr` in a helper is invisible to the built-in
patterns. Tell the scanner about it:

```cue
keyPatterns: ["mapErr\\([^,]+,\\s*\"([\\w.]+)\""]
```

One capture group, holding the key. A pattern with any other number is rejected
at load.

## Include the library's source

A layer that ships keys also references them. Leaving its source out of `scan`
makes every key it owns look unused:

```cue
scan: ["../web/app", "../../S99AB/src"]
```

## Skipped automatically

`node_modules`, `.git`, `.nuxt`, `.output`, `dist`, `vendor`, `bin`,
`testdata`, and the generated output directory - the generated bundles name
every key, so scanning them would make the unused report permanently empty.
