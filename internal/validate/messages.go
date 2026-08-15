// internal/validate/messages.go

package validate

// Messages is every diag key a Diagnostic can carry; a test checks cmd/loc renders each.
func Messages() []string {
	return []string{
		// parse, I18N001, I18N002, I18N004
		"fileTooLarge",
		"unreadable",
		"invalidYAML",
		"badPath",
		"bareValue",
		"duplicateKey",
		"valueAndPlural",
		"valueNotString",
		"badPlural",
		"descriptionNotString",
		"badPlaceholders",
		// validate, one entry per errorAt and warnAt site
		"override",
		"missingKey",
		"noPluralRule",
		"emptyTranslation",
		"pluralFormMissing",
		"pluralCategoryExtra",
		"pluralOtherMissing",
		"pluralArgumentMissing",
		"pluralArgumentType",
		"placeholderUndeclared",
		"placeholderUnused",
		"goFieldCollision",
		// compare, one entry per r.add site
		"kindMismatch",
		"pluralArgumentDiffers",
		"placeholderMismatch",
		"placeholderTypeMismatch",
	}
}
