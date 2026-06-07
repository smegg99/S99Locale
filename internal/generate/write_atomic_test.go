// internal/generate/write_atomic_test.go

package generate

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// failingFS fails the nth WriteTemp, so a test can put a full disk in the
// middle of a build.
type failingFS struct {
	osFS
	failOn int
	calls  int
}

var errDiskFull = errors.New("no space left on device")

func (f *failingFS) WriteTemp(dir string, body []byte) (string, error) {
	f.calls++
	if f.calls == f.failOn {
		return "", errDiskFull
	}
	return f.osFS.WriteTemp(dir, body)
}

func output(t *testing.T, dir string, files map[string]string) *Output {
	t.Helper()

	out := NewOutput(dir)
	for path, body := range files {
		out.Add(path, []byte(body))
	}
	return out
}

// A build that fails partway must leave every file at its old bytes.
func TestAFailedStageCommitsNothing(t *testing.T) {
	dir := t.TempDir()

	first := output(t, dir, map[string]string{
		"a.json": "old-a", "b.json": "old-b", "c.json": "old-c", "d.json": "old-d",
	})
	if _, _, err := first.Write(); err != nil {
		t.Fatal(err)
	}

	second := output(t, dir, map[string]string{
		"a.json": "new-a", "b.json": "new-b", "c.json": "new-c", "d.json": "new-d",
	})
	second.fs = &failingFS{failOn: 3}

	if _, _, err := second.Write(); !errors.Is(err, errDiskFull) {
		t.Fatalf("err = %v, want the injected failure", err)
	}

	for name, want := range map[string]string{
		"a.json": "old-a", "b.json": "old-b", "c.json": "old-c", "d.json": "old-d",
	} {
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(body) != want {
			t.Errorf("%s = %q, want %q - a failed build committed a file", name, body, want)
		}
	}
}

// The same failure must not delete anything either.
func TestAFailedStagePrunesNothing(t *testing.T) {
	dir := t.TempDir()

	if _, _, err := output(t, dir, map[string]string{"a.json": "a", "gone.json": "orphan"}).Write(); err != nil {
		t.Fatal(err)
	}

	// gone.json is not produced, so a successful build would prune it.
	next := output(t, dir, map[string]string{"a.json": "changed", "b.json": "b"})
	next.fs = &failingFS{failOn: 1}

	if _, _, err := next.Write(); !errors.Is(err, errDiskFull) {
		t.Fatalf("err = %v, want the injected failure", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gone.json")); err != nil {
		t.Fatal("a failed build pruned an orphan it never replaced")
	}
}

// Staged files must not survive a failure, or the next run's orphan sweep finds
// them and the tree accumulates debris.
func TestAFailedStageLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()

	out := output(t, dir, map[string]string{"a.json": "a", "b.json": "b", "c.json": "c"})
	out.fs = &failingFS{failOn: 3}

	if _, _, err := out.Write(); !errors.Is(err, errDiskFull) {
		t.Fatalf("err = %v, want the injected failure", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if len(e.Name()) >= len(tempPrefix) && e.Name()[:len(tempPrefix)] == tempPrefix {
			t.Fatalf("a staged file survived the failure: %s", e.Name())
		}
	}
}

// The orphan sweep must not treat another run's staging file as debris. With
// the lock held this cannot arise; without one it degrades to a stray file
// rather than a deleted file.
func TestTheOrphanSweepIgnoresStagingFiles(t *testing.T) {
	dir := t.TempDir()

	out := output(t, dir, map[string]string{"a.json": "a"})
	if _, _, err := out.Write(); err != nil {
		t.Fatal(err)
	}

	stray := filepath.Join(dir, tempPrefix+"999")
	if err := os.WriteFile(stray, []byte("mid-write"), 0o644); err != nil {
		t.Fatal(err)
	}

	orphans, err := out.orphans()
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range orphans {
		if o == filepath.Base(stray) {
			t.Fatal("the orphan sweep would delete a file another run is staging")
		}
	}
}
