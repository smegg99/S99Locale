// cmd/loc/clean_test.go

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanRemovesOnlyTheOutput(t *testing.T) {
	path := project(t, valid)
	a, _, _ := testApp(t, options{configPath: path})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Dir(path)
	if err := cmdClean(ctx, a); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "generated")); !os.IsNotExist(err) {
		t.Fatal("clean left the output behind")
	}
	// clean leaves the canonical source alone.
	if _, err := os.Stat(filepath.Join(dir, "locales", "en", "common.yaml")); err != nil {
		t.Fatal("clean removed the source")
	}
}

// A clean that was cancelled before it started removes nothing.
func TestCleanOnACancelledContextRemovesNothing(t *testing.T) {
	path := project(t, valid)
	a, _, _ := testApp(t, options{configPath: path})

	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := cmdClean(ctx, a); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "generated")); err != nil {
		t.Fatalf("a cancelled clean removed the output: %v", err)
	}
}

// Removing a mistyped path would be unrecoverable, so an output that resolves
// to the config's own directory is refused.
func TestCleanRefusesToRemoveTheConfigDirectory(t *testing.T) {
	dir := t.TempDir()

	if err := os.MkdirAll(filepath.Join(dir, "locales", "en"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "locales", "en", "a.yaml"),
		[]byte("x:\n  value: \"y\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "config.cue")
	body := strings.Replace(cliConfig, `output: "generated"`, `output: "."`, 1)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	a, _, _ := testApp(t, options{configPath: path})
	err := cmdClean(context.Background(), a)
	if err == nil {
		t.Fatal("clean accepted its own config directory as the output")
	}
	if !strings.Contains(err.Error(), "not below the config directory") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "locales")); statErr != nil {
		t.Fatal("the source was removed anyway")
	}
}

// `output: ".."` reaches outside the project entirely. RemoveAll on it took the
// canonical source and every sibling directory with it.
func TestCleanRefusesAnOutputAboveTheConfigDirectory(t *testing.T) {
	dir := t.TempDir()
	sibling := filepath.Join(dir, "precious")

	if err := os.MkdirAll(sibling, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibling, "keep.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	path := cleanProject(t, filepath.Join(dir, "i18n"), `output: ".."`)

	a, _, _ := testApp(t, options{configPath: path})
	err := cmdClean(context.Background(), a)
	if err == nil {
		t.Fatal("clean accepted an output above the config directory")
	}
	if !strings.Contains(err.Error(), "not below the config directory") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(sibling, "keep.txt")); statErr != nil {
		t.Fatal("clean removed a sibling of the config directory")
	}
}

// An absolute output elsewhere is the same mistake with a different spelling.
func TestCleanRefusesAnAbsoluteOutputElsewhere(t *testing.T) {
	elsewhere := t.TempDir()
	path := cleanProject(t, t.TempDir(), fmt.Sprintf("output: %q", elsewhere))

	a, _, _ := testApp(t, options{configPath: path})
	if err := cmdClean(context.Background(), a); err == nil {
		t.Fatal("clean accepted an absolute output outside the project")
	}
	if _, statErr := os.Stat(elsewhere); statErr != nil {
		t.Fatal("clean removed an unrelated directory")
	}
}

// Output pointed at a canonical root would delete the source it was built from.
func TestCleanRefusesAnOutputHoldingASourceRoot(t *testing.T) {
	path := cleanProject(t, t.TempDir(), `output: "."`)

	a, _, _ := testApp(t, options{configPath: path})
	if err := cmdClean(context.Background(), a); err == nil {
		t.Fatal("clean accepted an output holding the canonical root")
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(path), "locales", "en", "a.yaml")); statErr != nil {
		t.Fatal("clean removed the canonical source")
	}
}

// cleanProject writes a minimal project into dir, with output overridden.
func cleanProject(t *testing.T, dir, output string) string {
	t.Helper()

	full := filepath.Join(dir, "locales", "en", "a.yaml")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x:\n  value: \"y\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	body := strings.Replace(cliConfig, `output: "generated"`, output, 1)
	body = strings.Replace(body, `locales: ["en", "pl"]`, `locales: ["en"]`, 1)

	path := filepath.Join(dir, "config.cue")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCleanOnNothingIsFine(t *testing.T) {
	a, _, _ := testApp(t, options{configPath: project(t, valid)})

	if err := cmdClean(context.Background(), a); err != nil {
		t.Fatalf("err = %v", err)
	}
}
