# Shared messages

`loc lint shared` reports keys that may be one message. It never merges
anything.

## What earns high confidence

Every one of these, or it is downgraded:

- same message kind
- same text in every locale, not just the default one
- same placeholders, with the same types
- same plural argument and categories
- the same targets, resolved from each key's namespace
- at least two keys

All locales are compared. `file.open` and
`store.status.open` are both "Open" in English and `Otwórz` versus `Otwarte` in
Polish. Comparing one locale would report them as duplicates.

## Confidence levels

| level | meaning | shown |
|---|---|---|
| high | the whole contract matches everywhere | by default |
| medium | text matches, but descriptions, namespace targets or the placeholder contract differ | `--verbose` |
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

## Commands

```sh
loc lint shared            # high confidence only
loc lint shared --verbose  # everything, with reasons
loc lint duplicates        # exact contracts, whatever they are named
loc lint shared --json     # machine readable
```

A shared or duplicate finding never fails the command: these are suggestions.
`lint` still validates the locales first, so a broken source fails it under any
subcommand, and `--strict` makes a warning fail too.
