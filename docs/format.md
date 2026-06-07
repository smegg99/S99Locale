# Canonical format

## Layout

```
i18n/
├── config.cue
└── locales/
    ├── en/
    │   ├── common.yaml
    │   └── auth/
    │       └── login.yaml
    └── pl/
        └── ...
```

The path forms the key prefix. `en/auth/login.yaml` holding `title:` produces
`auth.login.title`. Files are never flattened, and the first segment is the
namespace that target filtering hangs off.

Two files must not reach the same key. A group named `login` inside `auth.yaml`
and a file at `auth/login.yaml` both produce `auth.login.*`. That is an error
(I18N004).

## Messages

A node holding `value` or `plural` is a message. Anything else is another level
of key.

```yaml
loginFailed:
  value: "Could not sign in as {email}"
  description: "Shown after an unsuccessful login attempt"
  placeholders:
    email: {type: string}
```

`description` becomes the generated Go accessor's doc comment.

### Placeholders

Canonical files use `{name}` only.

| target | form |
|---|---|
| vue-i18n | `{name}`, unchanged |
| go-i18n | `{{.Name}}` |

Every placeholder used must be declared, and every declared placeholder should
be used. The first is an error, the second a warning.

Names that differ only by an underscore collide once PascalCased for Go:
`user_id` and `userId` both become `.UserId`. That is an error (I18N015).

Types are `string`, `integer` or `number`, and must agree across locales.

### Plurals

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

`argument` names the counted placeholder, per message: real sources use both
`count` and `n`. It must be declared as an `integer` (I18N018) - the generated
Go passes it to go-i18n as the count.

A `zero` band is all locales or none (I18N019). The required categories differ
by locale on purpose; `zero` is a choice about the message, not about a
language.

A form may carry other placeholders too - `"{banks} banks and {registers}
registers"` is fine.

See [plurals.md](plurals.md) for which categories each locale requires.

### Metadata

```yaml
targets: [backend, web]     # overrides the namespace, does not add to it
                            # each must be a configured target (I18N006)
deprecated: true
sharing:
  isolate: true
  reason: "wording may diverge between contexts in other languages"
```

`sharing.isolate` suppresses shared-message suggestions. Under `--strict` it
needs a reason.

## Config

See [cli.md](cli.md) for the full config reference.
