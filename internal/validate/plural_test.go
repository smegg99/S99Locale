// internal/validate/plural_test.go

package validate_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/validate"
)

// Polish needs four bands.
func TestPolishPluralMissingBands(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": `
files:
  plural:
    argument: count
    forms: {one: "{count} file", other: "{count} files"}
  placeholders:
    count: {type: integer}
`,
		"pl/a.yaml": `
files:
  plural:
    argument: count
    forms: {one: "{count} plik", other: "{count} plików"}
  placeholders:
    count: {type: integer}
`})

	d := find(t, r, validate.CodePluralMissing)
	if d.Locale != "pl" {
		t.Fatalf("wrong locale flagged: %+v", d)
	}
	// few and many are both absent.
	count := 0
	for _, x := range r.Diagnostics {
		if x.Code == validate.CodePluralMissing {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("%d missing-band errors, want few and many", count)
	}
}

func TestExtraCategoryForTheLocale(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": `
files:
  plural:
    argument: count
    forms: {one: "a", few: "b", other: "c"}
  placeholders:
    count: {type: integer}
`,
		"pl/a.yaml": `
files:
  plural:
    argument: count
    forms: {one: "a", few: "b", many: "c", other: "d"}
  placeholders:
    count: {type: integer}
`})

	d := find(t, r, validate.CodePluralExtra)
	if d.Locale != "en" {
		t.Fatalf("English few should be unreachable, got %+v", d)
	}
}

func TestPluralArgumentMustMatchAcrossLocales(t *testing.T) {
	r := project(t, map[string]string{
		"en/a.yaml": `
files:
  plural:
    argument: count
    forms: {one: "{count} a", other: "{count} b"}
  placeholders:
    count: {type: integer}
`,
		"pl/a.yaml": `
files:
  plural:
    argument: n
    forms: {one: "{n} a", few: "{n} b", many: "{n} c", other: "{n} d"}
  placeholders:
    n: {type: integer}
`})

	if !has(r, validate.CodePluralArgument) {
		t.Fatalf("codes = %v", codes(r))
	}
}

// user_id and userId both become .UserId in Go.
func TestGoFieldCollision(t *testing.T) {
	body := "x:\n  value: \"{user_id} {userId}\"\n" +
		"  placeholders:\n    user_id: {type: string}\n    userId: {type: string}\n"

	r := project(t, map[string]string{"en/a.yaml": body, "pl/a.yaml": body})

	d := find(t, r, validate.CodeGoCollision)
	if !strings.Contains(d.Detail, "UserId") {
		t.Fatalf("detail does not name the field: %q", d.Detail)
	}
}

// Inner casing is preserved, so these stay distinct and must not be flagged.
func TestCasingAloneIsNotACollision(t *testing.T) {
	if got := validate.GoField("userID"); got != "UserID" {
		t.Fatalf("GoField(userID) = %q", got)
	}
	if got := validate.GoField("userId"); got != "UserId" {
		t.Fatalf("GoField(userId) = %q", got)
	}
	if validate.GoField("user_id") != validate.GoField("userId") {
		t.Fatal("user_id and userId should collide")
	}
}

func TestDiagnosticsAreSortedDeterministically(t *testing.T) {
	files := map[string]string{
		"en/a.yaml": "b:\n  value: \"x\"\na:\n  value: \"y\"\n",
		"pl/a.yaml": "",
	}

	first := codes(project(t, files))
	second := codes(project(t, files))
	if !slices.Equal(first, second) {
		t.Fatalf("two runs disagreed:\n%v\n%v", first, second)
	}
	if !slices.IsSorted(first) {
		t.Fatalf("codes are not sorted: %v", first)
	}
}

// A plural argument declared as a string is rejected.
func TestPluralArgumentMustBeAnInteger(t *testing.T) {
	body := `
files:
  plural:
    argument: count
    forms: {one: "{count} file", few: "{count} f", many: "{count} f", other: "{count} files"}
  placeholders:
    count: {type: string}
`
	r := project(t, map[string]string{"en/a.yaml": body, "pl/a.yaml": body})

	d := find(t, r, validate.CodePluralArgType)
	if !strings.Contains(d.Detail, "must be an integer") {
		t.Fatalf("detail = %q", d.Detail)
	}
}

func TestAnIntegerPluralArgumentIsFine(t *testing.T) {
	body := `
files:
  plural:
    argument: count
    forms: {one: "{count} file", few: "{count} f", many: "{count} f", other: "{count} files"}
  placeholders:
    count: {type: integer}
`
	r := project(t, map[string]string{"en/a.yaml": body, "pl/a.yaml": body})

	if has(r, validate.CodePluralArgType) {
		t.Fatalf("an integer argument was rejected: %v", codes(r))
	}
}
