// cmd/loc/cli_test.go

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

// run executes a command with stdout and stderr captured.
func run(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	realOut, realErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	runErr := fn()

	w.Close()
	os.Stdout, os.Stderr = realOut, realErr
	return <-done, runErr
}

var valid = map[string]string{
	"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
	"pl/common.yaml": "cancel:\n  value: \"Anuluj\"\n",
}

func TestBuildThenCheckPasses(t *testing.T) {
	path := project(t, valid)
	ctx := context.Background()

	if _, err := run(t, func() error { return cmdBuild(ctx, path, false) }); err != nil {
		t.Fatalf("build: %v", err)
	}
	out, err := run(t, func() error { return cmdCheck(ctx, path, false, false) })
	if err != nil {
		t.Fatalf("check after build: %v\n%s", err, out)
	}
}

// Output that has not been regenerated must fail check.
func TestCheckFailsOnStaleOutput(t *testing.T) {
	path := project(t, valid)
	ctx := context.Background()

	if _, err := run(t, func() error { return cmdBuild(ctx, path, false) }); err != nil {
		t.Fatal(err)
	}

	// Change a source without rebuilding.
	source := filepath.Join(filepath.Dir(path), "locales", "pl", "common.yaml")
	if err := os.WriteFile(source, []byte("cancel:\n  value: \"Zamknij\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, func() error { return cmdCheck(ctx, path, false, false) })
	if !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	if !strings.Contains(out, "stale") {
		t.Fatalf("output does not say why:\n%s", out)
	}
}

// check never writes, and fails on an unbuilt project: a missing file counts as stale.
func TestCheckNeverWrites(t *testing.T) {
	path := project(t, valid)
	ctx := context.Background()

	out, err := run(t, func() error { return cmdCheck(ctx, path, false, false) })
	if !errors.Is(err, errFailed) {
		t.Fatalf("check on an unbuilt project = %v, want a failure\n%s", err, out)
	}

	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "generated")); !os.IsNotExist(err) {
		t.Fatal("check created the output directory")
	}
}

func TestBuildFailsOnAMissingTranslation(t *testing.T) {
	path := project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"pl/common.yaml": "",
	})

	out, err := run(t, func() error { return cmdBuild(context.Background(), path, false) })
	if !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	if !strings.Contains(out, "I18N005") {
		t.Fatalf("no missing-key diagnostic:\n%s", out)
	}
}

// A failed validation leaves no half-generated tree.
func TestFailedBuildWritesNothing(t *testing.T) {
	path := project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"pl/common.yaml": "",
	})

	if _, err := run(t, func() error { return cmdBuild(context.Background(), path, false) }); err == nil {
		t.Fatal("expected a failure")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "generated")); !os.IsNotExist(err) {
		t.Fatal("a failed build left output behind")
	}
}

// Warnings pass normally and fail under --strict.
func TestStrictTurnsWarningsIntoFailures(t *testing.T) {
	files := map[string]string{
		"en/a.yaml": "x:\n  value: \"Hello\"\n  placeholders:\n    unused: {type: string}\n",
		"pl/a.yaml": "x:\n  value: \"Cześć\"\n  placeholders:\n    unused: {type: string}\n",
	}

	path := project(t, files)
	if _, err := run(t, func() error { return cmdBuild(context.Background(), path, false) }); err != nil {
		t.Fatalf("a warning failed an ordinary build: %v", err)
	}

	path = project(t, files)
	out, err := run(t, func() error { return cmdBuild(context.Background(), path, true) })
	if !errors.Is(err, errFailed) {
		t.Fatalf("strict did not fail on a warning: %v\n%s", err, out)
	}
}

func TestStatsReportsWhatIsThere(t *testing.T) {
	path := project(t, valid)

	out, err := run(t, func() error { return cmdStats(context.Background(), path) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"messages", "by namespace", "by target", "missing translations"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats missing %q:\n%s", want, out)
		}
	}
}

func TestUnknownConfigPathIsAnError(t *testing.T) {
	if _, err := run(t, func() error { return cmdBuild(context.Background(), "/nope/config.cue", false) }); err == nil {
		t.Fatal("a missing config was accepted")
	}
}

// An unknown subcommand is rejected.
func TestUnknownCommandIsRejected(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"nonsense"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("an unknown command was accepted")
	}
}

// refactor-shared requires --to, or it would silently plan nothing.
func TestRefactorRequiresADestination(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"refactor-shared", "a.b"})
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	if err := cmd.Execute(); err == nil {
		t.Fatal("refactor-shared ran without --to")
	}
}

// main maps errFailed to exit 1 and everything else to exit 2, so CI can tell a failing check
// from a broken job.
func TestStaleOutputAndBrokenConfigFailDifferently(t *testing.T) {
	ctx := context.Background()

	path := project(t, valid)
	_, stale := run(t, func() error { return cmdCheck(ctx, path, false, false) })
	if !errors.Is(stale, errFailed) {
		t.Fatalf("stale output = %v, want errFailed (exit 1)", stale)
	}

	_, broken := run(t, func() error { return cmdCheck(ctx, "/nope/config.cue", false, false) })
	if broken == nil {
		t.Fatal("a missing config was accepted")
	}
	if errors.Is(broken, errFailed) {
		t.Fatal("a missing config reported as a failed check (exit 1), want exit 2")
	}
}

func TestCheckJSONIsParseableAndStillFails(t *testing.T) {
	ctx := context.Background()
	path := project(t, valid)

	out, err := run(t, func() error { return cmdCheck(ctx, path, false, true) })
	if !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want errFailed", err)
	}

	var res CheckResult
	if jsonErr := json.Unmarshal([]byte(out), &res); jsonErr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jsonErr, out)
	}
	if res.Current {
		t.Fatal("current = true on an unbuilt project")
	}

	if _, err := run(t, func() error { return cmdBuild(ctx, path, false) }); err != nil {
		t.Fatal(err)
	}
	out, err = run(t, func() error { return cmdCheck(ctx, path, false, true) })
	if err != nil {
		t.Fatalf("check after build: %v\n%s", err, out)
	}
	if jsonErr := json.Unmarshal([]byte(out), &res); jsonErr != nil {
		t.Fatalf("output is not JSON: %v\n%s", jsonErr, out)
	}
	if !res.Current || res.Files == 0 {
		t.Fatalf("res = %+v", res)
	}
}

// A diagnostic reaches JSON with its code, severity and position intact.
func TestCheckJSONCarriesDiagnostics(t *testing.T) {
	path := project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"pl/common.yaml": "",
	})

	out, _ := run(t, func() error { return cmdCheck(context.Background(), path, false, true) })

	var res CheckResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	if res.Errors == 0 || len(res.Diagnostics) == 0 {
		t.Fatalf("res = %+v", res)
	}
	d := res.Diagnostics[0]
	if d.Code != "I18N005" || d.Severity != "error" || d.Source.File == "" {
		t.Fatalf("diagnostic = %+v", d)
	}
}
