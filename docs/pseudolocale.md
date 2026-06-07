# Pseudolocalisation

```cue
pseudolocale: true
```

Generates a `pseudo` locale alongside the real ones:

```
Sign in   ->   [Šïğñ ïñ··]
```

## What it catches

- **Hardcoded strings.** Anything that never went through i18n stays plain
  ASCII while everything around it is accented.
- **Layout overflow.** The output is 30% longer than the source, because German
  and Polish routinely are. A layout that only fits English is one that will
  break.
- **Truncation.** The brackets are the tell: a missing `]` is a clipped label.
- **Missing glyphs.** A font without the accented range shows boxes.

## What it preserves

Placeholders, exactly:

```
Hello {name}   ->   [Ĥéłłö {name}···]
```

A mangled `{name}` would stop interpolating, and the screen would then show a
bug this tool invented rather than one it found.

## Plurals

Bands come from the pseudolocale's own rule - two, like English - not from the
source's. Generating four Polish bands for a two-band locale would produce a
message no index function can address. A source band with no counterpart falls
back to `other`, and an opt-in `zero` is carried through.

## Rules

- Generated at build time, never written into the canonical source. Committing
  it would mean maintaining a translation nobody wrote.
- Exempt from cross-locale validation: it is complete by construction, so
  comparing it against the source would only ever report the padding.
- Deterministic. A pseudolocale that changed between builds would break the
  byte-identical guarantee everything else relies on.
- Off by default. In a production bundle it is dead weight.
- Selectable in the app's locale switcher. One nobody can reach tests nothing.
