# Plurals

A wrong plural band raises no error and logs nothing. The count reads in the
wrong grammatical form.

## Categories per locale

From CLDR, the same data go-i18n's rules are generated from.

| locale | required |
|---|---|
| en, de, es, fr, it, nl, pt, tr | one, other |
| pl, ru, uk, cs, sk | one, few, many, other |
| sl | one, two, few, other |
| ga | one, two, few, many, other |
| he | one, two, other |
| ja, zh, ko | other |

A language whose CLDR rules carry a real `zero` category - Arabic, Welsh,
Latvian - cannot be added as things stand. `zero` is reserved here for the
opt-in band, and one name cannot mean both.

Declaring a category a locale does not have is an error. A `few` in English is
text no rule will ever select.

A locale with no entry in the table is rejected at config load.

## The zero band

`zero` is an **opt-in extra band, not a CLDR category**.

No rule returns it for the locales above. Polish resolves a count of 0 to
`many`, so `0 głosów` is grammatically correct - but `brak głosów` usually
reads better, and there is no CLDR way to ask for that.

If one locale of a message declares `zero`, all must. A band present in one
language and absent in another is exactly the drift this tool exists to catch.

## How each runtime gets it

vue-i18n takes a pipe string, in band order:

```
brak głosów | {count} głos | {count} głosy | {count} głosów | {count} głosów
```

Order is `[zero?, one, two?, few?, many?, other]`, filtered to what the message
declares. vue-i18n only indexes into the bands, so `plurals.gen.ts` ships the
index function alongside:

```ts
const cats_pl = ['one', 'few', 'many', 'other'] as const
function plural_pl(choice: number, choicesLength: number): number {
  return band('pl', cats_pl, choice, choicesLength)
}

export const pluralRules: Record<string, PluralIndex> = {
  en: plural_en,
  pl: plural_pl,
}
```

The band order comes from the same table as the strings, so the two cannot
drift. Which band a count selects comes from `Intl.PluralRules`, which is CLDR
- the same thing go-i18n uses on the backend.

That part is not hand-written on purpose. A rule that dispatches on how many
categories a locale has rather than on the locale itself passes for Polish and
fails for its neighbours: Russian 21 is `one`, and Czech `many` is decimals
only, so every Czech count from 5 up belongs in `other`.

**go-i18n** takes native categories, and the zero band as a separate message:

```json
"tuner.reeds": {"one": "...", "few": "...", "many": "...", "other": "..."},
"tuner.reeds.zero": "brak głosów"
```

Only the generated accessor can reach it:

```go
if p.Count == 0 {
    return zeroForm(l, "tuner.reeds", data)
}
return withCount(l, "tuner.reeds", p.Count, data)
```

## The numbers that matter

Polish `few` is 2-4, but not 12-14. 22 is few; 12 is many. Russian adds that 21
is `one` where Polish makes it `many`. Czech and Slovak reserve `many` for
decimals, so 5 is `other` there and `many` in Polish.

Four locales give four different answers with the same category names, so
selection is delegated to the runtimes.
