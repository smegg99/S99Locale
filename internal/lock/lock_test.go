// internal/lock/lock_test.go

package lock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// flock is per open file description, not per process, so two AcquireAll calls
// in one test conflict exactly as two processes would.
func skipWithoutFlock(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		t.Skip("no advisory locking on this platform")
	}
}

func TestAnExclusiveLockExcludesAnother(t *testing.T) {
	skipWithoutFlock(t)
	dir := t.TempDir()

	held, err := AcquireAll(context.Background(), []string{dir}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	// AcquireAll waits, so go under it to assert the conflict without hanging.
	path, err := pathFor(dir)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if err := acquire(second, true); err == nil {
		t.Fatal("a second exclusive lock was granted while the first was held")
	}
}

func TestSharedLocksCoexist(t *testing.T) {
	skipWithoutFlock(t)
	dir := t.TempDir()

	first, err := AcquireAll(context.Background(), []string{dir}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	second, err := AcquireAll(context.Background(), []string{dir}, false, nil)
	if err != nil {
		t.Fatalf("two checks could not hold a shared lock at once: %v", err)
	}
	second.Release()
}

func TestReleaseFreesEveryLock(t *testing.T) {
	skipWithoutFlock(t)
	one, two := t.TempDir(), t.TempDir()

	first, err := AcquireAll(context.Background(), []string{one, two}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	first.Release()

	done := make(chan error, 1)
	go func() {
		held, err := AcquireAll(context.Background(), []string{one, two}, true, nil)
		held.Release()
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("re-acquiring after Release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Release did not free the locks")
	}
}

// A waiting run ends when its context is cancelled.
func TestAWaitEndsWithTheContext(t *testing.T) {
	skipWithoutFlock(t)
	dir := t.TempDir()

	first, err := AcquireAll(context.Background(), []string{dir}, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	waited := 0
	start := time.Now()
	if _, err := AcquireAll(ctx, []string{dir}, true, func() { waited++ }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if waited != 1 {
		t.Fatalf("onWait fired %d times, want exactly 1", waited)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("the wait took %v, so it is not interruptible", elapsed)
	}
}

// A set that fails on a later directory releases the ones it already took.
func TestAFailedSetReleasesWhatItTook(t *testing.T) {
	skipWithoutFlock(t)
	dirs, err := ordered([]string{t.TempDir(), t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	second, err := AcquireAll(context.Background(), dirs[1:], true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := AcquireAll(ctx, dirs, true, nil); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}

	path, err := pathFor(dirs[0])
	if err != nil {
		t.Fatal(err)
	}
	probe, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Close()

	if err := acquire(probe, true); err != nil {
		t.Fatalf("the first directory is still locked after the set failed: %v", err)
	}
}

// onWait fires once for the whole set, even when the run waits on two directories.
func TestAWaitIsAnnouncedOncePerSet(t *testing.T) {
	skipWithoutFlock(t)
	dirs, err := ordered([]string{t.TempDir(), t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}

	first, err := AcquireAll(context.Background(), dirs[:1], true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()
	second, err := AcquireAll(context.Background(), dirs[1:], true, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()

	// Freeing the first directory inside onWait makes the run reach the second every time.
	waited := 0
	onWait := func() {
		waited++
		first.Release()
	}
	if _, err := AcquireAll(ctx, dirs, true, onWait); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if waited != 1 {
		t.Fatalf("onWait fired %d times for one set, want 1", waited)
	}
}

// Two runs whose directory sets overlap partially would deadlock if each took
// them in its own order.
func TestOverlappingSetsDoNotDeadlock(t *testing.T) {
	skipWithoutFlock(t)
	one, two := t.TempDir(), t.TempDir()

	done := make(chan error, 2)
	for _, dirs := range [][]string{{one, two}, {two, one}} {
		go func() {
			for range 20 {
				held, err := AcquireAll(context.Background(), dirs, true, nil)
				if err != nil {
					done <- err
					return
				}
				held.Release()
			}
			done <- nil
		}()
	}

	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("two runs taking overlapping sets deadlocked")
		}
	}
}

func TestReleaseOnTheZeroValueIsSafe(t *testing.T) {
	var h *Held
	h.Release()
	(&Held{}).Release()

	var l *Lock
	l.release()
	(&Lock{}).release()
}

// Two configs writing to one output must serialise; two writing to different
// outputs must not.
func TestTheLockIsKeyedOnTheOutputDirectory(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()

	pa, err := pathFor(a)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := pathFor(b)
	if err != nil {
		t.Fatal(err)
	}
	if pa == pb {
		t.Fatal("different outputs share a lock file")
	}

	same, err := pathFor(filepath.Join(a, "..", filepath.Base(a)))
	if err != nil {
		t.Fatal(err)
	}
	if same != pa {
		t.Fatalf("the same directory spelled differently gave %s and %s", same, pa)
	}
}

// The lock is outside the output directory, since check must not create that directory.
func TestTheLockDoesNotLiveInTheOutputDirectory(t *testing.T) {
	skipWithoutFlock(t)
	out := filepath.Join(t.TempDir(), "generated")

	held, err := AcquireAll(context.Background(), []string{out}, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("taking a lock created the output directory")
	}
}
