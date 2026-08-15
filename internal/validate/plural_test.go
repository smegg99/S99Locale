// internal/validate/plural_test.go

package validate_test

import (
	"slices"
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
	// few and many are both absent, and each error names its band.
	var bands []any
	for _, x := range withMessage(r, "pluralFormMissing") {
		if x.Args["locale"] != "pl" {
			t.Fatalf("diagnostic = %+v; it does not name pl", x)
		}
		bands = append(bands, x.Args["category"])
	}
	if !slices.Equal(bands, []any{"few", "many"}) {
		t.Fatalf("missing bands = %v, want few and many", bands)
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
	if d.Message != "pluralCategoryExtra" || d.Args["locale"] != "en" || d.Args["category"] != "few" {
		t.Fatalf("diagnostic = %+v; it does not name en and few", d)
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

	if d := find(t, r, validate.CodePluralArgument); d.Message != "pluralArgumentDiffers" {
		t.Fatalf("message = %q", d.Message)
	}
}

// user_id and userId both become .UserId in Go.
func TestGoFieldCollision(t *testing.T) {
	body := "x:\n  value: \"{user_id} {userId}\"\n" +
		"  placeholders:\n    user_id: {type: string}\n    userId: {type: string}\n"

	r := project(t, map[string]string{"en/a.yaml": body, "pl/a.yaml": body})

	d := find(t, r, validate.CodeGoCollision)
	if d.Message != "goFieldCollision" || d.Args["first"] != "userId" || d.Args["second"] != "user_id" || d.Args["field"] != "UserId" {
		t.Fatalf("diagnostic = %+v; it does not name the field", d)
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
	// kind compares against a plain string: a message.PlaceholderType in Args renders empty.
	if d.Message != "pluralArgumentType" || d.Args["name"] != "count" || d.Args["kind"] != "string" {
		t.Fatalf("diagnostic = %+v; it does not name the argument and its type", d)
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
