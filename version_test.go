// version_test.go

package s99locale

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDeclaredComesFromTheVersionFile(t *testing.T) {
	body, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(body))

	if want == "" {
		t.Fatal("VERSION is empty")
	}
	if got := Declared(); got != want {
		t.Fatalf("Declared() = %q, want %q", got, want)
	}
	if strings.Count(want, ".") != 2 {
		t.Fatalf("VERSION = %q, want MAJOR.MINOR.PATCH", want)
	}
}

// The three shapes --version can print; a test binary only reaches the first through Version().
func TestVersionShapes(t *testing.T) {
	for _, c := range []struct {
		rev   string
		dirty bool
		want  string
	}{
		{rev: "", dirty: false, want: "0.1.0"},
		{rev: "a22e8d3", dirty: false, want: "0.1.0 (a22e8d3)"},
		{rev: "a22e8d3", dirty: true, want: "0.1.0 (a22e8d3, dirty)"},
	} {
		if got := format("0.1.0", c.rev, c.dirty); got != c.want {
			t.Errorf("format(0.1.0, %q, %v) = %q, want %q", c.rev, c.dirty, got, c.want)
		}
	}
}

func TestVersionStartsWithTheDeclaredVersion(t *testing.T) {
	if got := Version(); !strings.HasPrefix(got, Declared()) {
		t.Fatalf("Version() = %q, want it to start with %q", got, Declared())
	}
}

// A tagged commit's VERSION must match the tag go install resolves; untagged commits skip.
func TestTagAgreesWithVersionFile(t *testing.T) {
	tag, ok := exactTag()
	if !ok {
		t.Skip("HEAD carries no tag")
	}

	if want := "v" + Declared(); tag != want {
		t.Fatalf("HEAD is tagged %s but VERSION says %s; one of them is wrong",
			tag, Declared())
	}
}

// exactTag returns the tag on HEAD, or false outside a worktree, which is how go install builds.
func exactTag() (string, bool) {
	git, err := exec.LookPath("git")
	if err != nil {
		return "", false
	}
	if err := exec.Command(git, "rev-parse", "--git-dir").Run(); err != nil {
		return "", false
	}

	out, err := exec.Command(git, "describe", "--tags", "--exact-match").Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}
