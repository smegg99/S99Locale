// internal/generate/cldr_test.go

package generate_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/generate"
	"github.com/smegg99/s99locale/internal/parse"
	"github.com/smegg99/s99locale/internal/plural"
)

// The generated index function must pick the same band as go-i18n's CLDR rules for every locale.

// harness indexes each generated pipe string and compares the form with the CLDR category.
const harness = `
import { readFileSync } from 'node:fs'
import { pluralRules } from './plurals.gen.ts'

const cases = JSON.parse(readFileSync('cases.json', 'utf8'))
const bad: string[] = []

for (const [locale, spec] of Object.entries(cases) as [string, any][]) {
  const bands = spec.pipe.split('|').map((s: string) => s.trim())
  const select = new Intl.PluralRules(locale)
  const index = pluralRules[locale]

  if (!index) { bad.push(locale + ': no index function'); continue }
  if (bands.length !== spec.categories.length) {
    bad.push(locale + ': ' + bands.length + ' bands, ' + spec.categories.length + ' categories')
    continue
  }

  for (let n = 0; n <= 1000; n++) {
    const got = bands[index(n, bands.length)]
    const want = select.select(n)
    if (got !== want) bad.push(locale + ': n=' + n + ' rendered ' + got + ', CLDR says ' + want)
  }
}

if (bad.length) {
  console.error(bad.slice(0, 12).join('\n'))
  console.error(bad.length + ' mismatch(es)')
  process.exit(1)
}
`

func TestGeneratedIndexAgreesWithCLDRForEveryLocale(t *testing.T) {
	node := requireNode(t)

	locales := plural.Locales()
	out := localesProject(t, locales)

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "plurals.gen.ts"), file(t, out, "web/plurals.gen.ts"))
	writeFile(t, filepath.Join(dir, "cases.json"), []byte(casesJSON(t, out, locales)))
	writeFile(t, filepath.Join(dir, "harness.mts"), []byte(harness))

	if output, err := runNode(node, dir); err != nil {
		t.Fatalf("the generated index disagrees with CLDR:\n%s", output)
	}
}

func TestGeneratedGoPluralForms(t *testing.T) {
	counts := []int{0, 1, 2, 11, 20, 21, 22, 101}
	cases := map[string]string{
		"da": "other one other other other other other other",
		"el": "other one other other other other other other",
		"et": "other one other other other other other other",
		"fi": "other one other other other other other other",
		"hu": "other one other other other other other other",
		"lt": "other one few other other one few one",
		"lv": "zero one other zero zero one other one",
		"nb": "other one other other other other other other",
		"ro": "few one few few other other other few",
		"sv": "other one other other other other other other",
		"th": "other other other other other other other other",
	}
	out := localesProject(t, plural.Locales())
	for tag, expected := range cases {
		t.Run(tag, func(t *testing.T) {
			bundle := i18n.NewBundle(language.Make(tag))
			if _, err := bundle.ParseMessageFileBytes(file(t, out, "backend/locales/"+tag+".json"), tag+".json"); err != nil {
				t.Fatal(err)
			}
			localizer := i18n.NewLocalizer(bundle, tag)
			for i, want := range strings.Fields(expected) {
				got, err := localizer.Localize(&i18n.LocalizeConfig{MessageID: "app.files", PluralCount: counts[i]})
				if err != nil || got != want {
					t.Errorf("count %d: got %q (%v), want %q", counts[i], got, err, want)
				}
			}
		})
	}
}

// localesProject builds a project with one plural per locale, each form named after its category.
func localesProject(t *testing.T, locales []string) *generate.Outputs {
	t.Helper()
	dir := t.TempDir()

	for _, locale := range locales {
		rule, err := plural.For(locale)
		if err != nil {
			t.Fatalf("%s: %v", locale, err)
		}

		var forms strings.Builder
		for _, c := range rule.Required {
			fmt.Fprintf(&forms, "      %s: \"%s\"\n", c, c)
		}
		writeFile(t, filepath.Join(dir, "locales", locale, "app.yaml"),
			[]byte("files:\n  plural:\n    argument: count\n    forms:\n"+forms.String()+
				"  placeholders:\n    count: {type: integer}\n"))
	}

	quoted := make([]string, 0, len(locales))
	for _, l := range locales {
		quoted = append(quoted, `"`+l+`"`)
	}
	cfgPath := filepath.Join(dir, "config.cue")
	writeFile(t, cfgPath, fmt.Appendf(nil,
		"defaultLocale: \"en\"\nlocales: [%s]\ntargets: [\"web\", \"backend\"]\nroots: [\"locales\"]\noutput: \"generated\"\n",
		strings.Join(quoted, ", ")))

	cfg, err := config.Load(context.Background(), cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	res, err := parse.Roots(context.Background(), cfg.AbsRoots(), cfg.Locales)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Errors {
		t.Fatalf("fixture does not parse: %v", e)
	}

	out, err := generate.Build(context.Background(), cfg, res.Set)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// casesJSON takes each pipe string from the generated bundle, so the shipped band order is tested.
func casesJSON(t *testing.T, out *generate.Outputs, locales []string) string {
	t.Helper()

	var b strings.Builder
	b.WriteString("{")

	for i, locale := range locales {
		rule, err := plural.For(locale)
		if err != nil {
			t.Fatal(err)
		}

		bundle := jsonFile(t, out, fmt.Sprintf("web/%s/app.json", locale))
		app, ok := bundle["app"].(map[string]any)
		if !ok {
			t.Fatalf("%s: bundle = %v", locale, bundle)
		}
		pipe, ok := app["files"].(string)
		if !ok {
			t.Fatalf("%s: no pipe string in %v", locale, app)
		}

		cats := make([]string, 0, len(rule.Required))
		for _, c := range rule.Required {
			cats = append(cats, `"`+string(c)+`"`)
		}

		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, "%q:{\"pipe\":%q,\"categories\":[%s]}", locale, pipe, strings.Join(cats, ","))
	}

	b.WriteString("}")
	return b.String()
}

// nodeFloor matches the justfile's check-node recipe.
const nodeFloor = 24

func requireNode(t *testing.T) string {
	t.Helper()

	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; cannot check the generated index against CLDR")
	}

	out, err := exec.Command(node, "--version").Output()
	if err != nil {
		t.Skipf("node --version: %v", err)
	}
	if nodeMajor(string(out)) < nodeFloor {
		t.Skipf("node %s is below the %d floor", strings.TrimSpace(string(out)), nodeFloor)
	}
	return node
}

func nodeMajor(version string) int {
	n := 0
	for _, r := range strings.TrimPrefix(strings.TrimSpace(version), "v") {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func runNode(node, dir string) (string, error) {
	cmd := exec.Command(node, "harness.mts")
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeFile(t *testing.T, path string, body []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
