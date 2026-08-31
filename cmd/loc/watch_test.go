// cmd/loc/watch_test.go

package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/smegg99/s99locale/internal/catalog"
)

// A ctrl-c that ends a lock wait stops watch without a failure line.
func TestACancelledRebuildPrintsNoFailure(t *testing.T) {
	path := project(t, valid)
	holder := hold(t, filepath.Join(filepath.Dir(path), "generated", "backend"), true)
	defer holder.Release()

	a, out, _ := testApp(t, options{configPath: path})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.out = cancelOn{w: out, line: "loc: waiting for another loc run to finish", cancel: cancel}

	rebuild(ctx, a)

	got := out.String()
	if want := catalog.ReportWaitingForLock(a.say()); !strings.Contains(got, want) {
		t.Fatalf("output %q does not say it waited", got)
	}
	if blamed := catalog.ReportWatchFailed(a.say()); strings.Contains(got, blamed) {
		t.Fatalf("output %q blames the sources for a ctrl-c", got)
	}
}

// A rebuild that fails with an error cmdBuild did not print says why before it says it failed.
func TestAFailedRebuildSaysWhy(t *testing.T) {
	path := project(t, valid)
	a, out, _ := testApp(t, options{configPath: path})
	ctx := context.Background()

	// A half-typed config stops the next build with a load error that cmdBuild does not print.
	if err := os.WriteFile(path, []byte("targets: [\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	want := cmdBuild(ctx, a)
	if want == nil || errors.Is(want, errFailed) {
		t.Fatalf("build = %v, want an error cmdBuild does not print", want)
	}
	if out.Len() != 0 {
		t.Fatalf("cmdBuild printed %q; this test needs an error it does not print", out.String())
	}

	rebuild(ctx, a)

	got := out.String()
	why := strings.Index(got, want.Error())
	failed := strings.Index(got, catalog.ReportWatchFailed(a.say()))
	if why < 0 || failed < why {
		t.Fatalf("output %q does not say why the build failed before saying it failed", got)
	}
}

// cancelOn cancels a context when line is written, which is a ctrl-c at that moment.
type cancelOn struct {
	w      io.Writer
	line   string
	cancel context.CancelFunc
}

func (c cancelOn) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	if strings.Contains(string(p), c.line) {
		c.cancel()
	}
	return n, err
}
