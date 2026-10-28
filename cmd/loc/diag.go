// cmd/loc/diag.go

package main

import (
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/smegg99/s99term"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/validate"
)

// renderDiagnostic turns one finding into the lines a person reads.
func (a *app) renderDiagnostic(d validate.Diagnostic) string {
	l := a.say()

	var rows [][2]string
	if d.Key != "" {
		rows = append(rows, [2]string{catalog.DiagLabelKey(l), d.Key})
	}
	for _, p := range d.Context {
		rows = append(rows, [2]string{diagLabel(l, p), diagValue(l, p)})
	}
	if d.Cause != "" {
		rows = append(rows, [2]string{catalog.DiagLabelCause(l), d.Cause})
	}
	if d.Source.File != "" {
		rows = append(rows, [2]string{catalog.DiagLabelSource(l), d.Source.String()})
	}

	head := a.screen().Renderer().Line(diagStatus(d), diagText(l, d), s99term.F("code", d.Code))
	if len(rows) == 0 {
		return head + "\n"
	}
	return head + "\n" + a.pairs(rows) + "\n"
}

// diagStatus maps a severity onto the shared state vocabulary.
func diagStatus(d validate.Diagnostic) s99term.Status {
	if d.Severity == validate.SevWarning {
		return s99term.StatusWarning
	}
	return s99term.StatusFailed
}

// diagLabel resolves a context label: a locale code is data, a word is a key.
func diagLabel(l *i18n.Localizer, p validate.Pair) string {
	switch p.LabelKey {
	case "labelWas":
		return catalog.DiagLabelWas(l)
	case "labelNow":
		return catalog.DiagLabelNow(l)
	default:
		return p.Label
	}
}

// diagValue resolves a context value the same way diagLabel resolves a label.
func diagValue(l *i18n.Localizer, p validate.Pair) string {
	switch p.ValueKey {
	case "labelNone":
		return catalog.DiagLabelNone(l)
	default:
		return p.Value
	}
}

// diagText picks the sentence by message key, not code: one code has several sentences.
func diagText(l *i18n.Localizer, d validate.Diagnostic) string {
	switch d.Message {
	case "fileTooLarge":
		return catalog.DiagFileTooLarge(l, catalog.DiagFileTooLargeParams{
			Size: argInt(d.Args, "size"), Limit: argInt(d.Args, "limit")})
	case "unreadable":
		return catalog.DiagUnreadable(l)
	case "invalidYAML":
		return catalog.DiagInvalidYAML(l)
	case "badPath":
		return catalog.DiagBadPath(l)
	case "bareValue":
		return catalog.DiagBareValue(l)
	case "duplicateKey":
		return catalog.DiagDuplicateKey(l, catalog.DiagDuplicateKeyParams{At: argString(d.Args, "at")})
	case "valueAndPlural":
		return catalog.DiagValueAndPlural(l)
	case "valueNotString":
		return catalog.DiagValueNotString(l)
	case "badPlural":
		return catalog.DiagBadPlural(l)
	case "descriptionNotString":
		return catalog.DiagDescriptionNotString(l)
	case "badPlaceholders":
		return catalog.DiagBadPlaceholders(l)
	case "unknownField":
		return catalog.DiagUnknownField(l, catalog.DiagUnknownFieldParams{Field: argString(d.Args, "field")})
	case "override":
		return catalog.DiagOverride(l)
	case "missingKey":
		return catalog.DiagMissingKey(l, catalog.DiagMissingKeyParams{
			Locale: argString(d.Args, "locale"), Base: argString(d.Args, "base")})
	case "noPluralRule":
		return catalog.DiagNoPluralRule(l)
	case "emptyTranslation":
		return catalog.DiagEmptyTranslation(l)
	case "templateBraces":
		return catalog.DiagTemplateBraces(l)
	case "pluralFormMissing":
		return catalog.DiagPluralFormMissing(l, catalog.DiagPluralFormMissingParams{
			Locale: argString(d.Args, "locale"), Category: argString(d.Args, "category")})
	case "pluralCategoryExtra":
		return catalog.DiagPluralCategoryExtra(l, catalog.DiagPluralCategoryExtraParams{
			Locale: argString(d.Args, "locale"), Category: argString(d.Args, "category")})
	case "pluralOtherMissing":
		return catalog.DiagPluralOtherMissing(l)
	case "pluralArgumentMissing":
		return catalog.DiagPluralArgumentMissing(l)
	case "pluralArgumentType":
		return catalog.DiagPluralArgumentType(l, catalog.DiagPluralArgumentTypeParams{
			Name: argString(d.Args, "name"), Kind: argString(d.Args, "kind")})
	case "placeholderUndeclared":
		return catalog.DiagPlaceholderUndeclared(l, catalog.DiagPlaceholderUndeclaredParams{
			Name: argString(d.Args, "name")})
	case "placeholderUnused":
		return catalog.DiagPlaceholderUnused(l, catalog.DiagPlaceholderUnusedParams{
			Name: argString(d.Args, "name")})
	case "goFieldCollision":
		return catalog.DiagGoFieldCollision(l, catalog.DiagGoFieldCollisionParams{
			First: argString(d.Args, "first"), Second: argString(d.Args, "second"),
			Field: argString(d.Args, "field")})
	case "kindMismatch":
		return catalog.DiagKindMismatch(l)
	case "pluralArgumentDiffers":
		return catalog.DiagPluralArgumentDiffers(l)
	case "placeholderMismatch":
		return catalog.DiagPlaceholderMismatch(l)
	case "placeholderTypeMismatch":
		return catalog.DiagPlaceholderTypeMismatch(l, catalog.DiagPlaceholderTypeMismatchParams{
			Name: argString(d.Args, "name")})
	default:
		// Unmapped: print the raw key; diag_test.go fails on it.
		return "diag." + d.Message
	}
}

func argString(args map[string]any, name string) string {
	value, _ := args[name].(string)
	return value
}

// argInt also takes a float64, which is what a number becomes after a JSON round trip.
func argInt(args map[string]any, name string) int {
	switch value := args[name].(type) {
	case int:
		return value
	case float64:
		return int(value)
	default:
		return 0
	}
}
