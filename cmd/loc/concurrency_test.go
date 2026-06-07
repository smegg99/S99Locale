// cmd/loc/concurrency_test.go

package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Two builds racing on one output must not lose a file. Without the lock,
// orphan detection in one run reads a tree the other is still writing and can
// delete a file it thinks is debris.
func TestConcurrentBuildsDoNotLoseFiles(t *testing.T) {
	path := project(t, valid)
	ctx := context.Background()

	if _, err := run(t, func() error { return cmdBuild(ctx, path, false) }); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(filepath.Dir(path), "generated")
	want := countFiles(t, generated)

	// Race from a clean tree, or every run finds its output unchanged, writes
	// nothing, and the test proves nothing. Writing is where they collide:
	// writeAtomic stages a .loc-* temp file that another run's orphan sweep
	// sees as debris it did not produce.
	if err := os.RemoveAll(generated); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- cmdBuild(ctx, path, false)
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("a concurrent build failed: %v", err)
		}
	}

	if got := countFiles(t, generated); got != want {
		t.Fatalf("%d files after concurrent builds, want %d", got, want)
	}
	if _, err := run(t, func() error { return cmdCheck(ctx, path, false, false) }); err != nil {
		t.Fatalf("check after concurrent builds: %v", err)
	}
}

func countFiles(t *testing.T, dir string) int {
	t.Helper()
	n := 0

	err := filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			n++
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}
