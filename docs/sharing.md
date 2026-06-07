# Shared messages

`loc lint shared` reports keys that may be one message. It never merges
anything.

## What earns high confidence

Every one of these, or it is downgraded:

- same message kind
- same text in every locale, not just the default one
- same placeholders, with the same types
- same plural argument and categories
- same targets
- no `sharing.isolate`
- at least two keys

All locales are compared. `file.open` and
`store.status.open` are both "Open" in English and `Otwórz` versus `Otwarte` in
Polish. Comparing one locale would report them as duplicates.

## Confidence levels

| level | meaning | shown |
|---|---|---|
| high | the whole contract matches everywhere | by default |
| medium | text matches, but descriptions, targets or the placeholder contract differ | `--verbose` |
| low | only the default locale matches | `--verbose` |

Every downgrade carries a reason, so a medium finding says what to look at.

## Suggested names

Suggestions come from a short controlled vocabulary:

```
common.action.accept   common.action.cancel   common.action.close
common.action.retry    common.action.save     common.action.delete
common.answer.yes      common.answer.no       common.state.loading
common.navigation.back common.navigation.next
```

Text outside the vocabulary gets no suggestion, so `common` stays a closed list.

No suggestion is offered when:

- the text is not in the vocabulary
- the message has placeholders or is a plural, since it carries context a
  generic name would lose
- the suggested key already exists with a different contract

When one key in the group already lives in `common`, that key is suggested
instead of a new one.

## Intentional duplication

```yaml
dialog:
  accept:
    value: "OK"
    sharing:
      isolate: true
      reason: "may diverge from the menu wording in other languages"
```

Isolated keys are excluded from detection entirely. Under `--strict` the reason
is required - a suppression nobody explained is one nobody can review.

## Commands

```sh
loc lint shared            # high confidence only
loc lint shared --verbose  # everything, with reasons
loc lint duplicates        # exact contracts, whatever they are named
loc lint shared --json     # machine readable
```

Never exits non-zero. These are suggestions, not failures.
