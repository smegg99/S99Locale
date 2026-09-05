// internal/generate/ts.go

package generate

import (
	"fmt"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/plural"
)

const tsHeader = message.GeneratedHeader + "\n\n"

// The generated file names. Frontends import them by name.
const (
	pluralsFileName = "plurals.gen.ts"
	keysFileName    = "keys.gen.ts"
)

// Plurals writes the vue-i18n index function; Intl.PluralRules selects bands as go-i18n does.
func Plurals(out *Output, cfg *config.Config) error {
	var b strings.Builder

	b.WriteString(tsHeader)
	b.WriteString(pluralsPrelude)

	for _, locale := range cfg.Locales {
		rule, err := plural.For(locale)
		if err != nil {
			return err
		}
		b.WriteString(localeRule(locale, rule))
	}

	b.WriteString("\nexport const pluralRules: Record<string, PluralIndex> = {\n")
	for _, locale := range cfg.Locales {
		fmt.Fprintf(&b, "  '%s': %s,\n", locale, ruleFuncName(locale))
	}
	b.WriteString("}\n")

	out.Add(pluralsFileName, []byte(b.String()))
	return nil
}

// pluralsPrelude is the shared band arithmetic; each locale adds its category list and tag.
const pluralsPrelude = `/*
 * Band order per locale, matching the generated pipe strings exactly.
 *
 * vue-i18n hands us the count and how many bands the message actually has.
 * A message that declares fewer bands than its locale allows is clamped to
 * its own last band rather than reaching past the end.
 */

export type PluralIndex = (choice: number, choicesLength: number) => number

const selectors: Record<string, Intl.PluralRules> = {}

function select(tag: string, n: number): string {
  let s = selectors[tag]
  if (!s) {
    s = selectors[tag] = new Intl.PluralRules(tag)
  }
  return s.select(n)
}

// band maps a count onto a position in the bands a message declares.
function band(tag: string, cats: readonly string[], choice: number, choicesLength: number): number {
  const n = Math.abs(Math.floor(choice))

  // A category outside the table degrades to the last band rather than
  // indexing past the end.
  const i = cats.indexOf(select(tag, n))
  return Math.min(i < 0 ? cats.length - 1 : i, choicesLength - 1)
}
`

// localeRule emits one locale's category list and index function.
func localeRule(locale string, rule plural.Rule) string {
	var b strings.Builder

	suffix := identSuffix(locale)
	fmt.Fprintf(&b, "\nconst cats_%s = [%s] as const\n", suffix, quotedCategories(rule))
	fmt.Fprintf(&b, "function plural_%s(choice: number, choicesLength: number): number {\n", suffix)
	fmt.Fprintf(&b, "  return band('%s', cats_%s, choice, choicesLength)\n", locale, suffix)
	b.WriteString("}\n")

	return b.String()
}

func ruleFuncName(locale string) string { return "plural_" + identSuffix(locale) }

func identSuffix(locale string) string {
	return strings.NewReplacer("-", "_", ".", "_").Replace(locale)
}

func quotedCategories(rule plural.Rule) string {
	names := make([]string, 0, len(rule.Required))
	for _, c := range rule.Required {
		names = append(names, "'"+string(c)+"'")
	}
	return strings.Join(names, ", ")
}

// Keys writes the locale list and the default locale a frontend imports.
func Keys(out *Output, cfg *config.Config) error {
	var b strings.Builder
	b.WriteString(tsHeader)

	fmt.Fprintf(&b, "export const locales = %s as const\n", jsonArray(cfg.Locales))
	fmt.Fprintf(&b, "export const defaultLocale = '%s'\n", cfg.DefaultLocale)

	out.Add(keysFileName, []byte(b.String()))
	return nil
}

func jsonArray(in []string) string {
	quoted := make([]string, 0, len(in))
	for _, s := range in {
		quoted = append(quoted, "'"+s+"'")
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}
