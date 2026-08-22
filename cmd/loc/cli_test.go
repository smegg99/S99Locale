// cmd/loc/cli_test.go

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

	"github.com/smegg99/s99locale/internal/catalog"
)

// cliConfig builds web alone, so generated/backend stands in for a retired tree.
const cliConfig = `
defaultLocale: "en"
locales: ["en", "pl"]
targets: ["web"]
roots: ["locales"]
output: "generated"
`

// project writes a config and locale files, returning the config path.
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	for name, body := range files {
		full := filepath.Join(dir, "locales", name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(dir, "config.cue")
	if err := os.WriteFile(path, []byte(cliConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// testApp builds a command's state over two buffers instead of swapping the process's descriptors,
// which is unsafe under -race.
func testApp(t *testing.T, o options) (*app, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()

	out, data := &bytes.Buffer{}, &bytes.Buffer{}
	a := &app{out: out, data: data, opts: o}
	// English, whatever the machine running the tests has in its environment.
	a.localize(catalog.DefaultLocale)
	return a, out, data
}

var valid = map[string]string{
	"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
	"pl/common.yaml": "cancel:\n  value: \"Anuluj\"\n",
}

// onlyWarnings validates with a warning and no error: an unused placeholder.
var onlyWarnings = map[string]string{
	"en/a.yaml": "x:\n  value: \"Hello\"\n  placeholders:\n    unused: {type: string}\n",
	"pl/a.yaml": "x:\n  value: \"Cześć\"\n  placeholders:\n    unused: {type: string}\n",
}

func TestBuildThenCheckPasses(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := cmdCheck(ctx, a, false); err != nil {
		t.Fatalf("check after build: %v\n%s", err, out.String())
	}
}

// Output that has not been regenerated must fail check.
func TestCheckFailsOnStaleOutput(t *testing.T) {
	path := project(t, valid)
	a, out, _ := testApp(t, options{configPath: path})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}

	// Change a source without rebuilding.
	source := filepath.Join(filepath.Dir(path), "locales", "pl", "common.yaml")
	if err := os.WriteFile(source, []byte("cancel:\n  value: \"Zamknij\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := cmdCheck(ctx, a, false)
	if !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	if !strings.Contains(out.String(), "stale") {
		t.Fatalf("output does not say why:\n%s", out.String())
	}
}

// A quiet check that fails on stale output still names the heading and the files.
func TestQuietCheckStillSaysWhatIsStale(t *testing.T) {
	path := project(t, valid)
	a, out, _ := testApp(t, options{configPath: path, quiet: true})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(filepath.Dir(path), "locales", "pl", "common.yaml")
	if err := os.WriteFile(source, []byte("cancel:\n  value: \"Zamknij\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cmdCheck(ctx, a, false); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	for _, want := range []string{"generated output is stale", "  generated/web/pl/common.json\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--quiet hid %q:\n%s", want, out.String())
		}
	}
}

// A quiet check names a hand-edited orphan, and build refuses to remove it.
func TestQuietCheckStillSaysWhatWasModified(t *testing.T) {
	path := project(t, valid)
	a, out, _ := testApp(t, options{configPath: path, quiet: true})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(path)
	edited := filepath.Join(dir, "generated", "web", "pl", "common.json")
	if err := os.WriteFile(edited, []byte(`{"mine":"now"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Dropping pl leaves the edited file as an orphan the record names.
	body := strings.Replace(cliConfig, `locales: ["en", "pl"]`, `locales: ["en"]`, 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cmdCheck(ctx, a, false); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	for _, want := range []string{"loc cannot prove it wrote these, so build will not touch them; remove them by hand:\n  generated/web/pl/common.json\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--quiet hid %q:\n%s", want, out.String())
		}
	}

	err := cmdBuild(ctx, a)
	if err == nil || !strings.Contains(err.Error(), "modified since loc wrote it") {
		t.Fatalf("build on an edited orphan = %v, want a refusal", err)
	}
	if _, statErr := os.Stat(edited); statErr != nil {
		t.Fatal("build removed the edited file anyway")
	}
}

// check --json names a hand-edited orphan in its own field, and is not current.
func TestCheckJSONNamesWhatWasModified(t *testing.T) {
	path := project(t, valid)
	a, _, data := testApp(t, options{configPath: path})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	edited := filepath.Join(filepath.Dir(path), "generated", "web", "pl", "common.json")
	if err := os.WriteFile(edited, []byte(`{"mine":"now"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(cliConfig, `locales: ["en", "pl"]`, `locales: ["en"]`, 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cmdCheck(ctx, a, true); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	var res CheckResult
	if err := json.Unmarshal(data.Bytes(), &res); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, data.String())
	}
	if res.Current {
		t.Fatal("current = true with a modified file")
	}
	if len(res.Modified) != 1 || res.Modified[0] != "generated/web/pl/common.json" {
		t.Fatalf("modified = %v, want [generated/web/pl/common.json]", res.Modified)
	}
	if slices.Contains(res.Stale, "generated/web/pl/common.json") {
		t.Fatalf("stale = %v also names the modified file", res.Stale)
	}
}

// A build that fails after writing one tree still reports its removals, even under --quiet.
func TestAFailedBuildSaysWhatItRemoved(t *testing.T) {
	path := project(t, valid)
	both := strings.Replace(cliConfig, `targets: ["web"]`, `targets: ["web", "backend"]`, 1)
	if err := os.WriteFile(path, []byte(both), 0o644); err != nil {
		t.Fatal(err)
	}
	a, out, _ := testApp(t, options{configPath: path, quiet: true})
	ctx := context.Background()
	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}

	// web is written first; an unwritable backend directory fails the second tree.
	backend := filepath.Join(filepath.Dir(path), "generated", "backend")
	if err := os.Chmod(backend, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(backend, 0o755) })
	if probe, err := os.CreateTemp(backend, "probe"); err == nil {
		probe.Close()
		os.Remove(probe.Name())
		t.Skip("a read-only directory is still writable here")
	}

	narrowed := strings.Replace(both, `locales: ["en", "pl"]`, `locales: ["en"]`, 1)
	if err := os.WriteFile(path, []byte(narrowed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cmdBuild(ctx, a); err == nil {
		t.Fatal("build into a read-only tree succeeded")
	}
	if want := "loc: removed generated/web/pl/common.json\n"; !strings.Contains(out.String(), want) {
		t.Fatalf("the failed build hid %q:\n%s", want, out.String())
	}
}

// check never writes, and fails on an unbuilt project: a missing file counts as stale.
func TestCheckNeverWrites(t *testing.T) {
	path := project(t, valid)
	a, out, _ := testApp(t, options{configPath: path})

	if err := cmdCheck(context.Background(), a, false); !errors.Is(err, errFailed) {
		t.Fatalf("check on an unbuilt project = %v, want a failure\n%s", err, out.String())
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "generated")); !os.IsNotExist(err) {
		t.Fatal("check created the output directory")
	}
}

func TestBuildFailsOnAMissingTranslation(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"pl/common.yaml": "",
	})})

	if err := cmdBuild(context.Background(), a); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	if !strings.Contains(out.String(), "I18N005") {
		t.Fatalf("no missing-key diagnostic:\n%s", out.String())
	}
}

// Each diagnostic ends in a blank line, separating it from the next and the totals.
func TestReportSeparatesDiagnostics(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\nopen:\n  value: \"Open\"\n",
		"pl/common.yaml": "",
	})})

	if err := cmdBuild(context.Background(), a); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	for _, want := range []string{"\n\nI18N005", "\n\n2 error(s), 0 warning(s)\n"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("no blank line before %q:\n%s", strings.TrimSpace(want), out.String())
		}
	}
}

// A failed validation leaves no half-generated tree.
func TestFailedBuildWritesNothing(t *testing.T) {
	path := project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"pl/common.yaml": "",
	})
	a, _, _ := testApp(t, options{configPath: path})

	if err := cmdBuild(context.Background(), a); err == nil {
		t.Fatal("expected a failure")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "generated")); !os.IsNotExist(err) {
		t.Fatal("a failed build left output behind")
	}
}

// Warnings pass normally and fail under --strict.
func TestStrictTurnsWarningsIntoFailures(t *testing.T) {
	lenient, _, _ := testApp(t, options{configPath: project(t, onlyWarnings)})
	if err := cmdBuild(context.Background(), lenient); err != nil {
		t.Fatalf("a warning failed an ordinary build: %v", err)
	}

	strict, out, _ := testApp(t, options{configPath: project(t, onlyWarnings), strict: true})
	if err := cmdBuild(context.Background(), strict); !errors.Is(err, errFailed) {
		t.Fatalf("strict did not fail on a warning: %v\n%s", err, out.String())
	}
}

func TestStatsReportsWhatIsThere(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})

	if err := cmdStats(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"messages", "by namespace", "by target", "missing translations"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stats missing %q:\n%s", want, out.String())
		}
	}
}

// Each target writes its own tree, so the size table has one row per target.
func TestStatsSizesEachTarget(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})

	if err := cmdStats(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	_, sizes, ok := strings.Cut(out.String(), "generated size\n")
	if !ok {
		t.Fatalf("no size table:\n%s", out.String())
	}
	rows := strings.Split(strings.TrimSpace(sizes), "\n")
	if len(rows) != 2 || !strings.HasPrefix(strings.TrimSpace(rows[0]), "web ") || !strings.HasPrefix(strings.TrimSpace(rows[1]), "total ") {
		t.Fatalf("size rows = %q, want web then total", rows)
	}
}

// The stats report is the command's result, not a success line, so --quiet keeps it.
func TestQuietStatsKeepsTheReport(t *testing.T) {
	path := project(t, valid)

	loudApp, loud, _ := testApp(t, options{configPath: path})
	if err := cmdStats(context.Background(), loudApp); err != nil {
		t.Fatal(err)
	}
	quietApp, quiet, _ := testApp(t, options{configPath: path, quiet: true})
	if err := cmdStats(context.Background(), quietApp); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"locales", "messages"} {
		if !strings.Contains(quiet.String(), want) {
			t.Errorf("--quiet stats hid %q:\n%s", want, quiet.String())
		}
	}
	if quiet.String() != loud.String() {
		t.Errorf("--quiet changed the report:\n--- loud\n%s--- quiet\n%s", loud.String(), quiet.String())
	}
}

func TestUnknownConfigPathIsAnError(t *testing.T) {
	a, _, _ := testApp(t, options{configPath: "/nope/config.cue"})

	if err := cmdBuild(context.Background(), a); err == nil {
		t.Fatal("a missing config was accepted")
	}
}

// An unknown subcommand is rejected.
func TestUnknownCommandIsRejected(t *testing.T) {
	cmd := rootCmd(newApp(&bytes.Buffer{}, &bytes.Buffer{}))
	cmd.SetArgs([]string{"nonsense"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("an unknown command was accepted")
	}
}

// main maps errFailed to exit 1 and everything else to exit 2, so CI can tell a failing check
// from a broken job.
func TestStaleOutputAndBrokenConfigFailDifferently(t *testing.T) {
	ctx := context.Background()

	unbuilt, _, _ := testApp(t, options{configPath: project(t, valid)})
	if stale := cmdCheck(ctx, unbuilt, false); !errors.Is(stale, errFailed) {
		t.Fatalf("stale output = %v, want errFailed (exit 1)", stale)
	}

	missing, _, _ := testApp(t, options{configPath: "/nope/config.cue"})
	broken := cmdCheck(ctx, missing, false)
	if broken == nil {
		t.Fatal("a missing config was accepted")
	}
	if errors.Is(broken, errFailed) {
		t.Fatal("a missing config reported as a failed check (exit 1), want exit 2")
	}
}

func TestCheckJSONIsParseableAndStillFails(t *testing.T) {
	ctx := context.Background()
	a, _, data := testApp(t, options{configPath: project(t, valid)})

	if err := cmdCheck(ctx, a, true); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want errFailed", err)
	}

	var res CheckResult
	if jsonErr := json.Unmarshal(data.Bytes(), &res); jsonErr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jsonErr, data.String())
	}
	if res.Current {
		t.Fatal("current = true on an unbuilt project")
	}

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	// One JSON document per run: the first check's result is already read.
	data.Reset()
	if err := cmdCheck(ctx, a, true); err != nil {
		t.Fatalf("check after build: %v\n%s", err, data.String())
	}
	if jsonErr := json.Unmarshal(data.Bytes(), &res); jsonErr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jsonErr, data.String())
	}
	if !res.Current || res.Files == 0 {
		t.Fatalf("res = %+v", res)
	}
}

// A diagnostic reaches JSON with its code, severity and position intact.
func TestCheckJSONCarriesDiagnostics(t *testing.T) {
	a, _, data := testApp(t, options{configPath: project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"pl/common.yaml": "",
	})})

	_ = cmdCheck(context.Background(), a, true)

	var res CheckResult
	if err := json.Unmarshal(data.Bytes(), &res); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, data.String())
	}
	if res.Errors == 0 || len(res.Diagnostics) == 0 {
		t.Fatalf("res = %+v", res)
	}
	d := res.Diagnostics[0]
	if d.Code != "I18N005" || d.Severity != "error" || d.Source.File == "" {
		t.Fatalf("diagnostic = %+v", d)
	}
	// JSON carries the catalogue key and arguments, not the sentence.
	if d.Message != "missingKey" || d.Args["locale"] != "pl" || d.Args["base"] != "en" {
		t.Fatalf("diagnostic = %+v, want missingKey with its locales", d)
	}

	var raw struct {
		Diagnostics []map[string]json.RawMessage `json:"diagnostics"`
	}
	if err := json.Unmarshal(data.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw.Diagnostics[0]["detail"]; ok {
		t.Fatalf("a rendered sentence reached JSON:\n%s", data.String())
	}
}
