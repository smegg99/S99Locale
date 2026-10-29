// cmd/loc/concurrency_test.go

package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/smegg99/s99locale/internal/lock"
)

// Two builds racing on one output must not lose a file the other just wrote.
func TestConcurrentBuildsDoNotLoseFiles(t *testing.T) {
	path := project(t, valid)
	a, _, _ := testApp(t, options{configPath: path})
	ctx := context.Background()

	if err := cmdBuild(ctx, a); err != nil {
		t.Fatal(err)
	}
	generated := filepath.Join(filepath.Dir(path), "generated")
	want := countFiles(t, generated)

	// Race from an empty tree, or every run finds its output unchanged and writes nothing.
	if err := os.RemoveAll(generated); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			racer, _, _ := testApp(t, options{configPath: path})
			errs <- cmdBuild(ctx, racer)
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
	if err := cmdCheck(ctx, a, false); err != nil {
		t.Fatalf("check after concurrent builds: %v", err)
	}
}

// build clears a dropped target's tree and check reads it, so both wait for its holder, even quiet.
func TestBuildAndCheckWaitForARunOnARetiredTree(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(context.Context, *app) error
	}{
		{"build", cmdBuild},
		{"check", func(ctx context.Context, a *app) error { return cmdCheck(ctx, a, false) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := project(t, valid)
			generated := filepath.Join(filepath.Dir(path), "generated")
			holder := hold(t, filepath.Join(generated, "backend"), true)
			defer holder.Release()

			a, out, _ := testApp(t, options{configPath: path, quiet: true})
			if err := waitOut(tc.run, a); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v, want the wait to end with the context", err)
			}
			if !strings.Contains(out.String(), "loc: waiting for another loc run to finish") {
				t.Fatalf("output %q does not say it waited", out.String())
			}
			if _, err := os.Stat(generated); !os.IsNotExist(err) {
				t.Fatalf("a run that never got the lock touched the output: %v", err)
			}
		})
	}
}

// clean removes what every build tree's record names, so it waits for a build holding any of them.
func TestCleanWaitsForABuildInsideTheTree(t *testing.T) {
	for _, held := range []string{"web", "backend"} {
		t.Run(held, func(t *testing.T) {
			path := project(t, valid)
			a, _, _ := testApp(t, options{configPath: path})
			if err := cmdBuild(context.Background(), a); err != nil {
				t.Fatal(err)
			}

			generated := filepath.Join(filepath.Dir(path), "generated")
			holder := hold(t, filepath.Join(generated, held), true)
			defer holder.Release()

			if err := waitOut(cmdClean, a); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("err = %v, want the wait to end with the context", err)
			}
			if _, err := os.Stat(filepath.Join(generated, "web", "loc-manifest.json")); err != nil {
				t.Fatalf("clean removed a tree another run holds: %v", err)
			}
		})
	}
}

// check takes a shared lock, so a check never queues behind another check.
func TestCheckDoesNotWaitForAnotherCheck(t *testing.T) {
	path := project(t, valid)
	a, out, _ := testApp(t, options{configPath: path})
	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	out.Reset()

	holder := hold(t, filepath.Join(filepath.Dir(path), "generated", "web"), false)
	defer holder.Release()

	check := func(ctx context.Context, a *app) error { return cmdCheck(ctx, a, false) }
	if err := waitOut(check, a); err != nil {
		t.Fatalf("check behind another check: %v", err)
	}
	if strings.Contains(out.String(), "loc: waiting for another loc run to finish") {
		t.Fatalf("check waited for another check: %q", out.String())
	}
}

// clean locks exclusive, so it waits for a check holding the tree shared.
func TestCleanWaitsForACheck(t *testing.T) {
	path := project(t, valid)
	a, _, _ := testApp(t, options{configPath: path})
	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}

	web := filepath.Join(filepath.Dir(path), "generated", "web")
	holder := hold(t, web, false)
	defer holder.Release()

	if err := waitOut(cmdClean, a); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want the wait to end with the context", err)
	}
	if _, err := os.Stat(filepath.Join(web, "loc-manifest.json")); err != nil {
		t.Fatalf("clean removed a tree a check was reading: %v", err)
	}
}

// hold takes the lock another run would hold on dir: exclusive for a build, shared for a check.
func hold(t *testing.T, dir string, exclusive bool) *lock.Held {
	t.Helper()
	held, err := lock.AcquireAll(context.Background(), []string{dir}, exclusive, nil)
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// waitOut runs a command under a deadline that ends any wait for a lock.
func waitOut(run func(context.Context, *app) error, a *app) error {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return run(ctx, a)
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
