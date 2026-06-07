// internal/lock/lock_test.go

package lock

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// flock is per open file description, not per process, so two Acquire calls in
// one test conflict exactly as two processes would.
func skipWithoutFlock(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" || runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		t.Skip("no advisory locking on this platform")
	}
}

func TestAnExclusiveLockExcludesAnother(t *testing.T) {
	skipWithoutFlock(t)
	out := t.TempDir()

	held, err := Acquire(out, true)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()

	// Acquire blocks, so go under it to assert the conflict without hanging.
	path, err := pathFor(out)
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if err := acquire(second, true, false); err == nil {
		t.Fatal("a second exclusive lock was granted while the first was held")
	}
}

func TestSharedLocksCoexist(t *testing.T) {
	skipWithoutFlock(t)
	out := t.TempDir()

	first, err := Acquire(out, false)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	second, err := Acquire(out, false)
	if err != nil {
		t.Fatalf("two checks could not hold a shared lock at once: %v", err)
	}
	second.Release()
}

// Releasing must actually free it, or the second build of a watch session
// deadlocks against the first.
func TestReleaseFreesTheLock(t *testing.T) {
	skipWithoutFlock(t)
	out := t.TempDir()

	first, err := Acquire(out, true)
	if err != nil {
		t.Fatal(err)
	}
	first.Release()

	done := make(chan error, 1)
	go func() {
		l, err := Acquire(out, true)
		if l != nil {
			l.Release()
		}
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("re-acquiring after Release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Release did not free the lock")
	}
}

func TestReleaseOnTheZeroValueIsSafe(t *testing.T) {
	var l *Lock
	l.Release()
	(&Lock{}).Release()
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
	out := filepath.Join(t.TempDir(), "generated")

	l, err := Acquire(out, false)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Release()

	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("taking a lock created the output directory")
	}
}
