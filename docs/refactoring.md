# Refactoring

```sh
loc refactor-shared menu.accept dialog.accept --to common.action.accept
```

Prints a patch and a list of manual steps. **It never writes anything.**

```
Rename 2 key(s) to common.action.accept

--- a/web/app/layouts/default.vue
+++ b/web/app/layouts/default.vue
@@ line 5 @@
-menu.accept
+common.action.accept

Then, by hand:
  - remove menu.accept from i18n/locales/en/menu.yaml
  - remove dialog.accept from i18n/locales/en/dialog.yaml
  - add common.action.accept to each locale, with the text menu.accept already has

Nothing was written. Apply the patch, make the edits above, then run `loc check`.
```

## When it refuses

- **The messages are not the same.** Merging keys whose contracts differ would
  silently change one of them.
- **The destination exists and says something else.** Pick another name, or
  reconcile the two first.
- **A key is declared dynamic.** Its references cannot all be found, so the
  patch would be incomplete and look complete.
- **A key is missing from a locale.** There is no whole contract to compare.

## Why patch-only

A scan cannot see dynamic references, so a generated patch can be incomplete
and still look complete. A missed reference renders the raw key - visibly
wrong, but only on the screen nobody opened. Applying it by hand means someone
reads it first.
