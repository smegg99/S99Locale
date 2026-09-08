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

Declaring a category a locale does not have is an error. A `few` in English is
text no rule will ever select.

A locale with no entry in the table is rejected at config load.

## How each runtime gets it

vue-i18n takes a pipe string, in band order:

```
{count} głos | {count} głosy | {count} głosów | {count} głosów
```

Order is `[one, two?, few?, many?, other]`, filtered to what the message
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

The band order comes from the same table as the strings. Which band a count
selects comes from `Intl.PluralRules`, which is CLDR, as go-i18n is on the
backend.

A rule that dispatches on how many categories a locale has rather than on the
locale itself passes for Polish and fails for its neighbours: Russian 21 is `one`, and Czech `many` is decimals
only, so every Czech count from 5 up belongs in `other`.

go-i18n takes the native categories, one entry per band:

```json
"tuner.reeds": {"one": "...", "few": "...", "many": "...", "other": "..."}
```

The generated accessor passes the counted argument straight through:

```go
return withCount(l, "tuner.reeds", p.Count, data)
```

## The numbers that matter

Polish `few` is 2-4, but not 12-14. 22 is few; 12 is many. Russian adds that 21
is `one` where Polish makes it `many`. Czech and Slovak reserve `many` for
decimals, so 5 is `other` there and `many` in Polish.

Four locales give four different answers with the same category names, so
selection is delegated to the runtimes.
