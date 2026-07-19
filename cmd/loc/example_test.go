// cmd/loc/example_test.go

package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Builds the committed example end to end: every target, loadable output, and
// a second build that changes nothing.

func exampleConfig(t *testing.T) string {
	t.Helper()

	// Copied out, so a test never writes into the committed example.
	dir := t.TempDir()
	src := filepath.Join("..", "..", "examples", "basic")

	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(dir, rel)

		if info.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, body, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "i18n", "config.cue")
}

func TestExampleBuildsEveryTarget(t *testing.T) {
	path := exampleConfig(t)
	a, _, _ := testApp(t, options{configPath: path})

	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatalf("the committed example does not build: %v", err)
	}

	generated := filepath.Join(filepath.Dir(path), "generated")
	for _, want := range []string{
		"backend/en.json",
		"backend/pl.json",
		"web/en/app.json",
		"web/pl/app.json",
		"web/keys.gen.ts",
		"web/plurals.gen.ts",
		"go/messages_gen.go",
		"go/keys_gen.go",
		"manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(generated, want)); err != nil {
			t.Errorf("missing %s", want)
		}
	}
}

// The example output loads, not merely exists.
func TestExampleOutputIsLoadable(t *testing.T) {
	path := exampleConfig(t)
	a, _, _ := testApp(t, options{configPath: path})

	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(filepath.Dir(path), "generated")

	web := readJSON(t, filepath.Join(generated, "web", "pl", "app.json"))
	app, _ := web["app"].(map[string]any)
	if app["greeting"] != "Cześć {name}" {
		t.Fatalf("vue greeting = %v", app["greeting"])
	}
	// Polish: all four CLDR bands.
	files, _ := app["files"].(string)
	if n := strings.Count(files, "|"); n != 3 {
		t.Fatalf("%d separators in %q, want 3", n, files)
	}

	backend := readJSON(t, filepath.Join(generated, "backend", "pl.json"))
	if backend["emails.welcome.subject"] != "Witamy w {{.Product}}" {
		t.Fatalf("go form = %v", backend["emails.welcome.subject"])
	}
	// Target filtering: the web-only namespace is not in the backend bundle.
	if _, leaked := backend["app.greeting"]; leaked {
		t.Fatal("a web-only namespace reached the backend bundle")
	}
}

// A second build of the example changes nothing.
func TestExampleSecondBuildChangesNothing(t *testing.T) {
	path := exampleConfig(t)
	a, out, _ := testApp(t, options{configPath: path})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	before := snapshot(t, filepath.Join(filepath.Dir(path), "generated"))

	// Only the second build's line counts.
	out.Reset()
	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "0 changed") {
		t.Fatalf("the second build rewrote files:\n%s", out.String())
	}

	after := snapshot(t, filepath.Join(filepath.Dir(path), "generated"))
	for name, body := range before {
		if after[name] != body {
			t.Errorf("%s differs between builds", name)
		}
	}
	if len(before) != len(after) {
		t.Fatalf("%d files then %d", len(before), len(after))
	}

	// And check agrees the output is current.
	if err := cmdCheck(ctx, a, false); err != nil {
		t.Fatalf("check on a fresh build: %v", err)
	}
}

// The generated Go of the example parses.
func TestExampleGeneratedGoParses(t *testing.T) {
	path := exampleConfig(t)
	a, _, _ := testApp(t, options{configPath: path})

	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}

	src, err := os.ReadFile(filepath.Join(filepath.Dir(path), "generated", "go", "messages_gen.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	for _, want := range []string{
		"package messages",
		"type EmailsWelcomeSubjectParams struct",
		"Product string",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("generated Go missing %q", want)
		}
	}
	// app is web-only, so no accessor should exist for it.
	if strings.Contains(body, "func AppGreeting") {
		t.Error("a web-only message got a Go accessor")
	}
}

// go/format checks only syntax; a mistyped placeholder type fails only at compile time.
// The example is its own module with go-i18n, so it is compiled here.
func TestExampleGeneratedGoCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compiling the example needs the module cache")
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH")
	}

	path := exampleConfig(t)
	a, _, _ := testApp(t, options{configPath: path})
	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Dir(filepath.Dir(path))
	cmd := exec.Command(goTool, "build", "./...")
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err == nil {
		return
	}
	// A first run with no network and a cold cache cannot resolve go-i18n.
	if strings.Contains(string(out), "dial tcp") || strings.Contains(string(out), "no such host") {
		t.Skipf("cannot reach the module proxy:\n%s", out)
	}
	t.Fatalf("the generated Go does not compile:\n%s", out)
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return out
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out[rel] = string(body)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
