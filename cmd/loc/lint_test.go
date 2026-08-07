// cmd/loc/lint_test.go

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// scanLocales declares common.cancel and common.open; the scan source decides which are used.
var scanLocales = map[string]string{
	"en/common.yaml": "cancel:\n  value: \"Cancel\"\nopen:\n  value: \"Open\"\n",
	"pl/common.yaml": "cancel:\n  value: \"Anuluj\"\nopen:\n  value: \"Otwórz\"\n",
}

// findings references common.nope, which nobody declares, and leaves common.open unreferenced.
const findings = "t('common.cancel')\nt('common.nope')\n"

// scanProject writes the locales, a scan root and one source file with the given body.
func scanProject(t *testing.T, locales map[string]string, source string) string {
	t.Helper()

	path := project(t, locales)
	if err := os.WriteFile(path, []byte(cliConfig+"scan: [\"src\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := filepath.Join(filepath.Dir(path), "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "app.ts"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// Findings fail only under --strict; --json exits like the human form.
func TestLintKeysFailsOnAFindingOnlyUnderStrict(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		strict error
	}{
		{"clean", "t('common.cancel')\nt('common.open')\n", nil},
		{"unused only", "t('common.cancel')\n", errFailed},
		{"missing only", "t('common.cancel')\nt('common.open')\nt('common.nope')\n", errFailed},
		{"missing and unused", findings, errFailed},
	} {
		for _, asJSON := range []bool{false, true} {
			lenient, out, _ := testApp(t, options{configPath: scanProject(t, scanLocales, tc.source)})
			if err := cmdLintKeys(context.Background(), lenient, asJSON); err != nil {
				t.Errorf("%s, json=%v: failed without --strict: %v\n%s", tc.name, asJSON, err, out.String())
			}

			strict, out, _ := testApp(t, options{configPath: scanProject(t, scanLocales, tc.source), strict: true})
			if err := cmdLintKeys(context.Background(), strict, asJSON); !errors.Is(err, tc.strict) {
				t.Errorf("%s, json=%v: --strict = %v, want %v\n%s", tc.name, asJSON, err, tc.strict, out.String())
			}
		}
	}
}

// A finding is a diagnostic, so --quiet keeps it; only the success lines go.
func TestLintKeysFindingsSurviveQuiet(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: scanProject(t, scanLocales, findings), quiet: true})

	if err := cmdLintKeys(context.Background(), a, false); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"I18N030", "common.nope", "I18N032", "common.open", "dynamicKeys"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--quiet hid %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "scanned") {
		t.Fatalf("--quiet printed the scan summary:\n%s", out.String())
	}
}

// Every lint --json document goes to the data stream, none of it to the human one.
func TestLintJSONGoesToTheDataStream(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		run  func(context.Context, *app) error
	}{
		{"shared", "candidates", func(ctx context.Context, a *app) error { return cmdLintShared(ctx, a, false, true) }},
		{"keys", "missing", func(ctx context.Context, a *app) error { return cmdLintKeys(ctx, a, true) }},
		{"duplicates", "duplicates", func(ctx context.Context, a *app) error { return cmdLintDuplicates(ctx, a, true) }},
	} {
		a, out, data := testApp(t, options{configPath: scanProject(t, scanLocales, findings)})
		if err := tc.run(context.Background(), a); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}

		var doc map[string]json.RawMessage
		if err := json.Unmarshal(data.Bytes(), &doc); err != nil {
			t.Fatalf("%s: stdout is not JSON: %v\n%s", tc.name, err, data.String())
		}
		if _, ok := doc[tc.key]; !ok {
			t.Errorf("%s: no %q in %s", tc.name, tc.key, data.String())
		}
		if strings.Contains(out.String(), `"`+tc.key+`"`) {
			t.Errorf("%s: JSON reached the human stream:\n%s", tc.name, out.String())
		}
	}
}

// The strict JSON form through the tree: exit 1, and the document on stdout.
func TestLintKeysStrictJSONThroughTheTree(t *testing.T) {
	out, data := &bytes.Buffer{}, &bytes.Buffer{}
	root := rootCmd(newApp(out, data))
	root.SetArgs([]string{"lint", "keys", "--strict", "--json", "-c", scanProject(t, scanLocales, findings)})

	if err := root.Execute(); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want errFailed\n%s", err, out.String())
	}

	var res struct {
		Missing []struct{ Key string } `json:"missing"`
	}
	if err := json.Unmarshal(data.Bytes(), &res); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, data.String())
	}
	if len(res.Missing) != 1 || res.Missing[0].Key != "common.nope" {
		t.Fatalf("missing = %+v, want common.nope", res.Missing)
	}
}

// backendConfig builds the backend with output under the scan root; outputs is empty or a redirect.
func backendConfig(outputs string) string {
	return `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["backend"]
roots: ["locales"]
output: "generated"
scan: ["."]
go: {package: "messages"}
` + outputs
}

// Retired trees and a redirected target's generated files are ignored; a hand-written file is not.
func TestLintKeysCountsOnlySource(t *testing.T) {
	path := project(t, scanLocales)
	dir := filepath.Dir(path)

	write := func(rel, body string) {
		t.Helper()
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	build := func(wrote string) {
		t.Helper()
		a, out, _ := testApp(t, options{configPath: path})
		if err := cmdBuild(context.Background(), a); err != nil {
			t.Fatalf("build: %v\n%s", err, out.String())
		}
		if _, err := os.Stat(filepath.Join(dir, wrote, "keys_gen.go")); err != nil {
			t.Fatalf("the build wrote no keys_gen.go into %s: %v", wrote, err)
		}
	}
	unused := func() []string {
		t.Helper()
		a, out, data := testApp(t, options{configPath: path})
		if err := cmdLintKeys(context.Background(), a, true); err != nil {
			t.Fatalf("lint keys: %v\n%s", err, out.String())
		}
		var res struct {
			Unused []string `json:"unused"`
		}
		if err := json.Unmarshal(data.Bytes(), &res); err != nil {
			t.Fatalf("stdout is not JSON: %v\n%s", err, data.String())
		}
		return res.Unused
	}

	write("config.cue", backendConfig(""))
	build("generated/backend")
	write("src/app.ts", "t('common.cancel')\n")

	// Every keys_gen.go names KeyCommonOpen, so common.open stays unused until source names it.
	write("config.cue", backendConfig(`outputs: {backend: "src/messages"}`+"\n"))
	if got := unused(); !slices.Equal(got, []string{"common.open"}) {
		t.Fatalf("with a retired tree on disk: unused = %v, want [common.open]", got)
	}

	build("src/messages")
	if got := unused(); !slices.Equal(got, []string{"common.open"}) {
		t.Fatalf("with generated files in the redirected target: unused = %v, want [common.open]", got)
	}

	write("src/messages/label.go", "package messages\n\nvar openKey = KeyCommonOpen\n")
	if got := unused(); len(got) != 0 {
		t.Fatalf("the hand-written file in the redirected target was not read: unused = %v", got)
	}
}

// lowConfidence has one low-confidence candidate: "Open" twice in English, two words in Polish.
var lowConfidence = map[string]string{
	"en/common.yaml": "open:\n  value: \"Open\"\nopened:\n  value: \"Open\"\n",
	"pl/common.yaml": "open:\n  value: \"Otwórz\"\nopened:\n  value: \"Otwarty\"\n",
}

// --verbose exists on lint shared alone; bare lint hides low confidence and names lint shared.
func TestLintSharedVerboseThroughTheTree(t *testing.T) {
	lint, _, err := rootCmd(&app{}).Find([]string{"lint"})
	if err != nil {
		t.Fatal(err)
	}
	if lint.Flags().Lookup("verbose") != nil || lint.PersistentFlags().Lookup("verbose") != nil {
		t.Fatal("--verbose is on lint, so lint keys and lint duplicates inherit it")
	}

	path := project(t, lowConfidence)
	run := func(args ...string) (string, error) {
		out := &bytes.Buffer{}
		root := rootCmd(newApp(out, &bytes.Buffer{}))
		root.SetArgs(append(args, "-c", path))
		err := root.Execute()
		return out.String(), err
	}

	for _, args := range [][]string{{"lint"}, {"lint", "keys"}, {"lint", "duplicates"}} {
		out, err := run(append(args, "--verbose")...)
		if err == nil || errors.Is(err, errFailed) || !strings.Contains(err.Error(), "--verbose") {
			t.Errorf("%s --verbose: err = %v, want an unknown-flag error\n%s", strings.Join(args, " "), err, out)
		}
	}

	for _, args := range [][]string{{"lint"}, {"lint", "shared"}} {
		out, err := run(args...)
		if err != nil {
			t.Fatalf("%s: %v\n%s", strings.Join(args, " "), err, out)
		}
		if strings.Contains(out, "I18N021") {
			t.Errorf("%s showed a low-confidence finding:\n%s", strings.Join(args, " "), out)
		}
		if !strings.Contains(out, "loc lint shared --verbose") {
			t.Errorf("%s does not point at --verbose:\n%s", strings.Join(args, " "), out)
		}
	}

	shown, err := run("lint", "shared", "--verbose")
	if err != nil {
		t.Fatalf("lint shared --verbose: %v\n%s", err, shown)
	}
	if !strings.Contains(shown, "I18N021") {
		t.Errorf("lint shared --verbose hid the finding:\n%s", shown)
	}
}

// An interrupt after the load stops the scan walk with the reason.
func TestLintKeysScanStopsOnACancelledContext(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: scanProject(t, onlyWarnings, "t('a.x')\n")})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The totals line is the last thing the load prints before the scan starts.
	a.out = cancelOn{w: out, line: "warning(s)", cancel: cancel}

	if err := cmdLintKeys(ctx, a, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "scanned") {
		t.Fatalf("the scan ran to the end:\n%s", out.String())
	}
}
