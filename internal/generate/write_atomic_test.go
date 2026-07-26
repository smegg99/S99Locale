// internal/generate/write_atomic_test.go

package generate

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/message"
)

// failingFS fails the nth WriteTemp, Rename or Remove, to simulate a failure mid-build.
type failingFS struct {
	osFS
	failOn       int
	calls        int
	failRenameOn int
	renames      int
	failRemoveOn int
	removes      int
}

var errDiskFull = errors.New("no space left on device")

func (f *failingFS) WriteTemp(dir string, body []byte) (string, error) {
	f.calls++
	if f.calls == f.failOn {
		return "", errDiskFull
	}
	return f.osFS.WriteTemp(dir, body)
}

func (f *failingFS) Rename(from, to string) error {
	f.renames++
	if f.renames == f.failRenameOn {
		return errDiskFull
	}
	return f.osFS.Rename(from, to)
}

func (f *failingFS) Remove(path string) error {
	// discard removes temp files through here too; only a committed file counts.
	if strings.HasPrefix(filepath.Base(path), tempPrefix) {
		return f.osFS.Remove(path)
	}
	f.removes++
	if f.removes == f.failRemoveOn {
		return errDiskFull
	}
	return f.osFS.Remove(path)
}

// output stages files with the record a build leaves beside them.
func output(t *testing.T, dir string, files map[string]string) *Output {
	t.Helper()

	out := NewOutput(dir, "generated/web", message.TargetWeb)
	man := Manifest{SchemaVersion: ManifestSchemaVersion, Target: string(message.TargetWeb), Files: map[string]string{}}
	for path, body := range files {
		out.Add(path, []byte(body))
		man.Files[path] = hashOf([]byte(body))
	}
	if err := out.AddJSON(ManifestName, man); err != nil {
		t.Fatal(err)
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

// Staged files must not survive a failure: no record names a .loc-* file, so no build removes one.
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

// A failure after the record was staged must discard it too.
func TestAFailureAfterTheRecordIsStagedLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()

	// a.json, loc-manifest.json, z.json: the third write fails after the record's.
	out := output(t, dir, map[string]string{"a.json": "a", "z.json": "z"})
	out.fs = &failingFS{failOn: 3}

	if _, _, err := out.Write(); !errors.Is(err, errDiskFull) {
		t.Fatalf("err = %v, want the injected failure", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Fatalf("a failed first build left %s behind", e.Name())
	}
}

// A refusal reads as a sentence whether it names one file or several.
func TestARefusalReadsForOneFileAndForSeveral(t *testing.T) {
	for _, tc := range []struct {
		edited, foreign []string
		want            string
	}{
		{[]string{"a"}, nil, "generate: a was modified since loc wrote it; remove it by hand"},
		{[]string{"a", "b"}, nil, "generate: a, b were modified since loc wrote them; remove them by hand"},
		{nil, []string{"c"}, "generate: c is not in loc's record, so loc will not overwrite it; remove it by hand"},
		{nil, []string{"c", "d"}, "generate: c, d are not in loc's record, so loc will not overwrite them; remove them by hand"},
		{[]string{"a"}, []string{"c"}, "generate: a was modified since loc wrote it; c is not in loc's record, so loc will not overwrite it; remove them by hand"},
	} {
		if err := refusal(tc.edited, tc.foreign); err == nil || err.Error() != tc.want {
			t.Errorf("refusal(%v, %v) = %v, want %q", tc.edited, tc.foreign, err, tc.want)
		}
	}
	if err := refusal(nil, nil); err != nil {
		t.Errorf("refusal with nothing to refuse = %v", err)
	}
}

// recorded reads the record on disk and returns the paths it names.
func recorded(t *testing.T, dir string) []string {
	t.Helper()

	raw, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	var man Manifest
	if err := json.Unmarshal(raw, &man); err != nil {
		t.Fatal(err)
	}
	var names []string
	for rel := range man.Files {
		names = append(names, rel)
	}
	slices.Sort(names)
	return names
}

// A build that fails before its removals keeps the old record, so a rerun finishes them.
func TestACrashBeforeTheRecordLeavesTheOldOne(t *testing.T) {
	for name, fs := range map[string]*failingFS{
		"rename": {failRenameOn: 2},
		"remove": {failRemoveOn: 1},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if _, _, err := output(t, dir, map[string]string{"a.json": "a", "gone.json": "orphan"}).Write(); err != nil {
				t.Fatal(err)
			}

			next := map[string]string{"a.json": "changed", "b.json": "b"}
			crashed := output(t, dir, next)
			crashed.fs = fs
			if _, _, err := crashed.Write(); !errors.Is(err, errDiskFull) {
				t.Fatalf("err = %v, want the injected failure", err)
			}

			if got, want := recorded(t, dir), []string{"a.json", "gone.json"}; !slices.Equal(got, want) {
				t.Fatalf("the record on disk names %v after the crash, want the old %v", got, want)
			}
			if _, err := os.Stat(filepath.Join(dir, "gone.json")); err != nil {
				t.Fatal("the orphan went before the record that names it was replaced")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), tempPrefix) {
					t.Fatalf("a staged file survived the crash: %s", e.Name())
				}
			}

			_, removed, err := output(t, dir, next).Write()
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(removed, []string{"generated/web/gone.json"}) {
				t.Fatalf("the rerun removed %v, want the orphan", removed)
			}
			if got, want := recorded(t, dir), []string{"a.json", "b.json"}; !slices.Equal(got, want) {
				t.Fatalf("the record names %v after the rerun, want %v", got, want)
			}
		})
	}
}

// Pruning follows no symlink: not out of the tree, and not onto the link itself.
func TestPruneEmptiedLeavesASymlinkedDirectoryAlone(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "pkg")
	outside := filepath.Join(base, "outside")
	for _, dir := range []string{root, filepath.Join(outside, "sub"), filepath.Join(root, "real")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "evil")); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "real"), filepath.Join(root, "alias")); err != nil {
		t.Fatal(err)
	}

	emptied := []string{filepath.Join(root, "evil", "sub"), filepath.Join(root, "alias")}
	if err := pruneEmptied(root, emptied); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{
		filepath.Join(outside, "sub"),
		filepath.Join(root, "evil"),
		filepath.Join(root, "alias"),
		filepath.Join(root, "real"),
	} {
		if _, err := os.Lstat(keep); err != nil {
			t.Errorf("%s was pruned: %v", keep, err)
		}
	}
}

// removeIfEmpty follows no symlink: a link to an empty directory and the directory both stay.
func TestRemoveIfEmptyLeavesASymlinkAlone(t *testing.T) {
	base := t.TempDir()
	empty := filepath.Join(base, "empty")
	if err := os.Mkdir(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(empty, link); err != nil {
		t.Skipf("no symlinks here: %v", err)
	}

	if err := removeIfEmpty(link); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{link, empty} {
		if _, err := os.Lstat(keep); err != nil {
			t.Errorf("%s was removed: %v", keep, err)
		}
	}
}
