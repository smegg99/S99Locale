// cmd/loc/diag_test.go

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/validate"
)

// fixtureArgs gives every diagnostic argument a distinct value, so reading the wrong one shows.
var fixtureArgs = map[string]any{
	"size": 8, "limit": 4,
	"at":     "i18n/locales/en/auth.yaml:12",
	"locale": "pl", "base": "en",
	"category": "few",
	"name":     "count", "kind": "string",
	"first": "userId", "second": "userID", "field": "UserID",
}

// pascal keys Args the way the generated accessors key their template data.
func pascal(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for name, value := range args {
		out[validate.GoField(name)] = value
	}
	return out
}

// catalogueKeys is every key in loc's own default-locale source, sorted.
func catalogueKeys(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()

	cfg, err := config.Load(ctx, filepath.Join("..", "..", "i18n", "config.cue"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.Roots(ctx, cfg.AbsRoots(), []string{cfg.DefaultLocale})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) > 0 {
		t.Fatalf("loc's own catalogue does not parse: %v", res.Errors)
	}
	return res.Set.Keys()
}

// missingFrom is every name in want that got lacks.
func missingFrom(got, want []string) []string {
	var out []string
	for _, name := range want {
		if !slices.Contains(got, name) {
			out = append(out, name)
		}
	}
	return out
}

// diagText has to reach each key's entry and fill every placeholder; the compiler checks neither.
func TestEveryDiagnosticMessageRendersInEveryLocale(t *testing.T) {
	data := pascal(fixtureArgs)
	for _, key := range validate.Messages() {
		for _, locale := range catalog.Locales {
			l := catalog.Localizer(locale)
			want, err := l.Localize(&i18n.LocalizeConfig{MessageID: "diag." + key, TemplateData: data})
			if err != nil {
				t.Errorf("%s in %s has no catalogue entry: %v", key, locale, err)
				continue
			}

			got := diagText(l, validate.Diagnostic{Code: "I18N000", Severity: validate.SevError, Message: key, Args: fixtureArgs})
			switch {
			case got == "diag."+key:
				t.Errorf("%s in %s renders its own id; the switch has no case for it", key, locale)
			case got != want:
				t.Errorf("%s in %s renders %q, want %q; the case calls the wrong accessor, reads the wrong argument or leaves a placeholder unfilled", key, locale, got, want)
			}
		}
	}
}

// Every diag sentence has a registry key and every key a sentence; label keys are context words.
func TestTheRegistryIsTheCatalogue(t *testing.T) {
	var sentences []string
	for _, key := range catalogueKeys(t) {
		if name, ok := strings.CutPrefix(key, "diag."); ok && !strings.HasPrefix(name, "label") {
			sentences = append(sentences, name)
		}
	}
	registry := validate.Messages()

	if extra := missingFrom(registry, sentences); len(extra) > 0 {
		t.Errorf("diag.yaml has sentences validate.Messages does not list: %v", extra)
	}
	if missing := missingFrom(sentences, registry); len(missing) > 0 {
		t.Errorf("validate.Messages lists keys diag.yaml has no sentence for: %v", missing)
	}
	if len(registry) != len(slices.Compact(slices.Sorted(slices.Values(registry)))) {
		t.Errorf("validate.Messages lists a key twice: %v", registry)
	}
}

// A third-party parser's own words stay untranslated inside the frame.
func TestACauseIsKeptUntranslated(t *testing.T) {
	a, _, _ := testApp(t, options{})
	// Same package, so the localizer is set directly.
	a.words = catalog.Localizer("pl")

	got := a.renderDiagnostic(validate.Diagnostic{
		Code: validate.CodeYAML, Severity: validate.SevError,
		Message: "invalidYAML",
		Cause:   "yaml: line 3: mapping values are not allowed in this context",
		Source:  message.Source{File: "a.yaml", Line: 3},
	})
	if !strings.Contains(got, "mapping values are not allowed") {
		t.Fatalf("the parser's own words were lost:\n%s", got)
	}
	if !strings.Contains(got, "nieprawidłowy YAML") {
		t.Fatalf("the frame was not translated:\n%s", got)
	}
}

// labelled matches one rendered context line: the label, its colon, the padding, then the value.
func labelled(label, value string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(label+":") + `\s+` + value)
}

// validate names context words by key; a key the renderer does not know prints an empty label.
func TestContextWordsRenderInEveryLocale(t *testing.T) {
	path := project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Dismiss\"\n" +
			"greet:\n  value: \"Hi {name}\"\n  placeholders:\n    name: {type: string}\n",
		"pl/common.yaml": "cancel:\n  value: \"Zamknij\"\ngreet:\n  value: \"Cześć\"\n",
	})
	dir := filepath.Dir(path)
	for name, body := range map[string]string{
		"lib/en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"lib/pl/common.yaml": "cancel:\n  value: \"Anuluj\"\n",
		"config.cue":         strings.Replace(cliConfig, `roots: ["locales"]`, `roots: ["lib", "locales"]`, 1),
	} {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a, _, _ := testApp(t, options{configPath: path})
	_, _, report, _ := load(context.Background(), a)
	if report == nil {
		t.Fatal("no report")
	}

	shadowed := regexp.QuoteMeta(filepath.Join(dir, "lib", "en", "common.yaml"))
	winner := regexp.QuoteMeta(filepath.Join(dir, "locales", "en", "common.yaml"))
	for _, locale := range catalog.Locales {
		l := catalog.Localizer(locale)
		a.words = l

		var override, mismatch string
		var pairs []validate.Pair
		for _, d := range report.Diagnostics {
			switch d.Code {
			case validate.CodeOverride:
				override += a.renderDiagnostic(d)
				pairs = append(pairs, d.Context...)
			case validate.CodePlaceholder:
				mismatch += a.renderDiagnostic(d)
				pairs = append(pairs, d.Context...)
			}
		}
		// The earlier root was shadowed by the later one, so each label carries its own root's path.
		if !labelled(catalog.DiagLabelWas(l), shadowed).MatchString(override) {
			t.Errorf("%s: the override's %q line does not name the shadowed lib file:\n%s", locale, catalog.DiagLabelWas(l), override)
		}
		if !labelled(catalog.DiagLabelNow(l), winner).MatchString(override) {
			t.Errorf("%s: the override's %q line does not name the winning locales file:\n%s", locale, catalog.DiagLabelNow(l), override)
		}
		if !strings.Contains(mismatch, catalog.DiagLabelNone(l)) {
			t.Errorf("%s: an empty placeholder set does not say %q:\n%s", locale, catalog.DiagLabelNone(l), mismatch)
		}
		// A table cell cannot show a missing label, so the label is checked where it is resolved.
		for _, p := range pairs {
			if diagLabel(l, p) == "" {
				t.Errorf("%s: %+v renders no label:\n%s%s", locale, p, override, mismatch)
			}
		}
	}
}

// unreachable are the sites no fixture can reach, each with what stops the input first.
var unreachable = map[string]string{
	"badPath":               "filepath.Rel cannot fail for a file WalkDir found under its own root",
	"noPluralRule":          "config.Load rejects a locale with no plural rule",
	"pluralArgumentMissing": "parse rejects a plural with no argument as badPlural",
}

// everySite writes a project raising every reachable diagnostic site, returning its config path.
func everySite(t *testing.T) string {
	t.Helper()
	path := project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Dismiss\"\n" +
			"greet:\n  value: \"Hello {name}\"\n  placeholders:\n    name: {type: string}\n",
		"pl/common.yaml": "cancel:\n  value: \"Zamknij\"\n" +
			"greet:\n  value: \"Cześć {imie}\"\n  placeholders:\n    imie: {type: string}\n",
		"en/a.yaml": `
only:
  value: "Only in English"
files:
  plural:
    argument: count
    forms: {one: "{count} file", few: "{count} files", other: "{count} files"}
  placeholders:
    count: {type: integer}
noOther:
  plural:
    argument: count
    forms: {one: "{count} item"}
  placeholders:
    count: {type: integer}
typed:
  plural:
    argument: total
    forms: {one: "{total} a", other: "{total} b"}
  placeholders:
    total: {type: string}
undeclared:
  value: "Hi {who}"
unused:
  value: "Hi"
  placeholders:
    ghost: {type: string}
collide:
  value: "{user_id} {userId}"
  placeholders:
    user_id: {type: string}
    userId: {type: string}
typeDiff:
  value: "{amount}"
  placeholders:
    amount: {type: integer}
kind:
  value: "plain"
arg:
  plural:
    argument: count
    forms: {one: "{count} a", other: "{count} b"}
  placeholders:
    count: {type: integer}
empty:
  value: "Full"
`,
		"pl/a.yaml": `
files:
  plural:
    argument: count
    forms: {one: "{count} plik", other: "{count} plików"}
  placeholders:
    count: {type: integer}
noOther:
  plural:
    argument: count
    forms: {one: "{count} rzecz", few: "{count} rzeczy", many: "{count} rzeczy"}
  placeholders:
    count: {type: integer}
typed:
  plural:
    argument: total
    forms: {one: "{total} a", few: "{total} b", many: "{total} c", other: "{total} d"}
  placeholders:
    total: {type: string}
undeclared:
  value: "Cześć {who}"
unused:
  value: "Cześć"
  placeholders:
    ghost: {type: string}
collide:
  value: "{user_id} {userId}"
  placeholders:
    user_id: {type: string}
    userId: {type: string}
typeDiff:
  value: "{amount}"
  placeholders:
    amount: {type: string}
kind:
  plural:
    argument: count
    forms: {one: "a", few: "b", many: "c", other: "d"}
  placeholders:
    count: {type: integer}
arg:
  plural:
    argument: n
    forms: {one: "{n} a", few: "{n} b", many: "{n} c", other: "{n} d"}
  placeholders:
    n: {type: integer}
empty:
  value: "   "
`,
		"en/b.yaml": `
bare: "x"
both:
  value: "x"
  plural:
    argument: count
    forms: {other: "y"}
num:
  value: 3
badPlural:
  plural: "x"
desc:
  value: "x"
  description: 3
ph:
  value: "x"
  placeholders: "x"
`,
		"en/auth.yaml":       "login:\n  title:\n    value: \"from the group\"\n",
		"en/auth/login.yaml": "title:\n  value: \"from the file\"\n",
		"pl/broken.yaml":     "this: [is: not: yaml\n",
	})
	dir := filepath.Dir(path)

	for name, body := range map[string]string{
		"lib/en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"lib/pl/common.yaml": "cancel:\n  value: \"Anuluj\"\n",
		"config.cue":         strings.Replace(cliConfig, `roots: ["locales"]`, `roots: ["lib", "locales"]`, 1),
		// One MB past the limit, so the size and the limit render as different numbers.
		"locales/en/big.yaml": strings.Repeat("x", parse.MaxFileSize+1<<20),
	} {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// A dangling link is a .yaml file that cannot be read, even for root.
	if err := os.Symlink(filepath.Join(dir, "nowhere.yaml"), filepath.Join(dir, "locales", "en", "gone.yaml")); err != nil {
		t.Fatal(err)
	}
	return path
}

// Every construction site raises a registered key; a mistyped one renders its own id.
func TestEverySiteRaisesARegisteredMessage(t *testing.T) {
	a, _, _ := testApp(t, options{configPath: everySite(t)})
	_, _, report, _ := load(context.Background(), a)
	if report == nil {
		t.Fatal("no report")
	}

	var raised []string
	for _, d := range report.Diagnostics {
		if !slices.Contains(validate.Messages(), d.Message) {
			t.Errorf("%s %s raised %q, which validate.Messages does not list", d.Code, d.Key, d.Message)
		}
		if got := diagText(a.say(), d); got == "diag."+d.Message {
			t.Errorf("%s %s renders its own id %q", d.Code, d.Key, got)
		}
		if !slices.Contains(raised, d.Message) {
			raised = append(raised, d.Message)
		}
	}

	var reachable []string
	for _, key := range validate.Messages() {
		if _, ok := unreachable[key]; !ok {
			reachable = append(reachable, key)
		}
	}
	if missing := missingFrom(raised, reachable); len(missing) > 0 {
		t.Errorf("no site raised %v; a site's key drifted or the fixture lost a case", missing)
	}
	for key, why := range unreachable {
		if slices.Contains(raised, key) {
			t.Errorf("%s was raised, so it is no longer unreachable (%s)", key, why)
		}
	}
}

// Each site's arguments reach the slot its accessor names; key, context and source lines are real.
func TestEverySiteFillsItsSentence(t *testing.T) {
	path := everySite(t)
	dir := filepath.Dir(path)
	a, _, _ := testApp(t, options{configPath: path})
	_, _, report, _ := load(context.Background(), a)
	if report == nil {
		t.Fatal("no report")
	}

	rendered := func(msg, key string) string {
		var b strings.Builder
		for _, d := range report.Diagnostics {
			if d.Message == msg && d.Key == key {
				b.WriteString(a.renderDiagnostic(d))
			}
		}
		return b.String()
	}

	for _, locale := range catalog.Locales {
		l := catalog.Localizer(locale)
		a.words = l

		for _, c := range []struct{ msg, key, want string }{
			{"missingKey", "a.only", catalog.DiagMissingKey(l, catalog.DiagMissingKeyParams{Locale: "pl", Base: "en"})},
			{"pluralFormMissing", "a.files", catalog.DiagPluralFormMissing(l, catalog.DiagPluralFormMissingParams{Locale: "pl", Category: "few"})},
			{"pluralFormMissing", "a.files", catalog.DiagPluralFormMissing(l, catalog.DiagPluralFormMissingParams{Locale: "pl", Category: "many"})},
			{"pluralCategoryExtra", "a.files", catalog.DiagPluralCategoryExtra(l, catalog.DiagPluralCategoryExtraParams{Locale: "en", Category: "few"})},
			{"pluralArgumentType", "a.typed", catalog.DiagPluralArgumentType(l, catalog.DiagPluralArgumentTypeParams{Name: "total", Kind: "string"})},
			{"placeholderUndeclared", "a.undeclared", catalog.DiagPlaceholderUndeclared(l, catalog.DiagPlaceholderUndeclaredParams{Name: "who"})},
			{"placeholderUnused", "a.unused", catalog.DiagPlaceholderUnused(l, catalog.DiagPlaceholderUnusedParams{Name: "ghost"})},
			{"goFieldCollision", "a.collide", catalog.DiagGoFieldCollision(l, catalog.DiagGoFieldCollisionParams{First: "userId", Second: "user_id", Field: "UserId"})},
			{"placeholderTypeMismatch", "a.typeDiff", catalog.DiagPlaceholderTypeMismatch(l, catalog.DiagPlaceholderTypeMismatchParams{Name: "amount"})},
			{"duplicateKey", "auth.login.title", catalog.DiagDuplicateKey(l, catalog.DiagDuplicateKeyParams{At: filepath.Join(dir, "locales", "en", "auth.yaml") + ":2:3"})},
			{"fileTooLarge", "", catalog.DiagFileTooLarge(l, catalog.DiagFileTooLargeParams{Size: parse.MaxFileSize>>20 + 1, Limit: parse.MaxFileSize >> 20})},
		} {
			if got := rendered(c.msg, c.key); !strings.Contains(got, c.want) {
				t.Errorf("%s: %s on %q does not say %q:\n%s", locale, c.msg, c.key, c.want, got)
			}
		}

		// The mismatch shows both sides, the key and the file.
		mismatch := rendered("placeholderMismatch", "common.greet")
		for _, re := range []*regexp.Regexp{
			labelled("en", regexp.QuoteMeta("{name}")+"$"),
			labelled("pl", regexp.QuoteMeta("{imie}")+"$"),
			labelled(catalog.DiagLabelKey(l), regexp.QuoteMeta("common.greet")+"$"),
			labelled(catalog.DiagLabelSource(l), `\S*`+regexp.QuoteMeta(filepath.Join("pl", "common.yaml"))+`:\d+`),
		} {
			if !re.MatchString(mismatch) {
				t.Errorf("%s: the placeholder mismatch has no line matching %s:\n%s", locale, re, mismatch)
			}
		}

		broken := rendered("invalidYAML", "")
		if !strings.Contains(broken, catalog.DiagInvalidYAML(l)) || !labelled(catalog.DiagLabelCause(l), `\S`).MatchString(broken) {
			t.Errorf("%s: invalid YAML lost its sentence or the parser's cause:\n%s", locale, broken)
		}
	}
}

// check --json carries the parser's own words for a file that would not parse.
func TestCheckJSONCarriesTheParsersCause(t *testing.T) {
	a, _, data := testApp(t, options{configPath: everySite(t)})
	_ = cmdCheck(context.Background(), a, true)

	var res CheckResult
	if err := json.Unmarshal(data.Bytes(), &res); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, data.String())
	}
	for _, d := range res.Diagnostics {
		if d.Message == "invalidYAML" {
			if d.Code != validate.CodeYAML || d.Cause == "" || !strings.HasSuffix(d.Source.File, "broken.yaml") {
				t.Fatalf("diagnostic = %+v, want I18N001 on broken.yaml with the parser's cause", d)
			}
			return
		}
	}
	t.Fatalf("no invalidYAML in:\n%s", data.String())
}
