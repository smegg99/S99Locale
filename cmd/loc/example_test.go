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

// exampleConfig copies the example without its generated output, so a build writes every file.
func exampleConfig(t *testing.T) string {
	t.Helper()

	path := exampleCopy(t)
	example := filepath.Dir(filepath.Dir(path))
	for _, generated := range []string{filepath.Join("i18n", "generated"), "messages"} {
		if err := os.RemoveAll(filepath.Join(example, generated)); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

// exampleCopy is a copy of the example as committed, generated output included.
func exampleCopy(t *testing.T) string {
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

// The committed output is what a build of the committed sources writes, byte for byte.
func TestExampleCommittedOutputIsCurrent(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: exampleCopy(t)})

	if err := cmdCheck(context.Background(), a, false); err != nil {
		t.Fatalf("the committed example output is not current: %v\n%s", err, out.String())
	}
}

func TestExampleBuildsEveryTarget(t *testing.T) {
	path := exampleConfig(t)
	a, out, _ := testApp(t, options{configPath: path})

	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatalf("the committed example does not build: %v", err)
	}
	if want := "loc: 10 file(s) generated, 10 changed"; !strings.Contains(out.String(), want) {
		t.Fatalf("build said %q, want %q", out.String(), want)
	}

	cfgDir := filepath.Dir(path)
	for _, want := range []string{
		"generated/web/en/app.json",
		"generated/web/pl/app.json",
		"generated/web/keys.gen.ts",
		"generated/web/plurals.gen.ts",
		"generated/web/loc-manifest.json",
		"../messages/locales/en.json",
		"../messages/locales/pl.json",
		"../messages/messages_gen.go",
		"../messages/keys_gen.go",
		"../messages/loc-manifest.json",
	} {
		if _, err := os.Stat(filepath.Join(cfgDir, filepath.FromSlash(want))); err != nil {
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
	cfgDir := filepath.Dir(path)

	web := readJSON(t, filepath.Join(cfgDir, "generated", "web", "pl", "app.json"))
	app, _ := web["app"].(map[string]any)
	if app["greeting"] != "Cześć {name}" {
		t.Fatalf("vue greeting = %v", app["greeting"])
	}
	// Polish: all four CLDR bands.
	files, _ := app["files"].(string)
	if n := strings.Count(files, "|"); n != 3 {
		t.Fatalf("%d separators in %q, want 3", n, files)
	}

	backend := readJSON(t, filepath.Join(cfgDir, "..", "messages", "locales", "pl.json"))
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
	// The whole example copy, so both the web tree and the Go package are covered.
	example := filepath.Dir(filepath.Dir(path))
	before := snapshot(t, example)

	// Only the second build's line counts.
	out.Reset()
	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "0 changed") {
		t.Fatalf("the second build rewrote files:\n%s", out.String())
	}

	after := snapshot(t, example)
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

	src, err := os.ReadFile(filepath.Join(filepath.Dir(path), "..", "messages", "messages_gen.go"))
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
