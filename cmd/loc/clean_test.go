// cmd/loc/clean_test.go

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanRemovesOnlyTheOutput(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Dir(a.opts.configPath)

	out.Reset()
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
	// Each removed file is named.
	if !strings.Contains(out.String(), "loc: removed generated/web/loc-manifest.json\n") {
		t.Fatalf("out = %q", out.String())
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

// A clean that fails after clearing one tree still reports its removals, even under --quiet.
func TestAFailedCleanSaysWhatItRemoved(t *testing.T) {
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

	// web is cleared first; an unremovable backend directory fails the second tree.
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

	if err := cmdClean(ctx, a); err == nil {
		t.Fatal("clean of a read-only tree succeeded")
	}
	if want := "loc: removed generated/web/loc-manifest.json\n"; !strings.Contains(out.String(), want) {
		t.Fatalf("the failed clean hid %q:\n%s", want, out.String())
	}
}

func TestCleanOnNothingIsFine(t *testing.T) {
	a, out, _ := testApp(t, options{configPath: project(t, valid)})

	if err := cmdClean(context.Background(), a); err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(out.String(), "nothing to clean") {
		t.Fatalf("out = %q", out.String())
	}
}

// A tree loc has no record of stays, whatever it is named.
func TestCleanLeavesATreeItNeverRecorded(t *testing.T) {
	a, _, _ := testApp(t, options{configPath: project(t, valid)})
	dir := filepath.Dir(a.opts.configPath)

	stray := filepath.Join(dir, "generated", "web", "keys.gen.ts")
	if err := os.MkdirAll(filepath.Dir(stray), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stray, []byte("export type K = string\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := cmdClean(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatal("clean removed a file it had no record of")
	}
}
