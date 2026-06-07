// cmd/loc/quiet_test.go

package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// --quiet hides only success lines; a quiet run that fails still says why.
func TestQuietKeepsDiagnosticsAndFailure(t *testing.T) {
	path := project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"pl/common.yaml": "",
	})

	quiet = true
	defer func() { quiet = false }()

	out, err := run(t, func() error { return cmdBuild(context.Background(), path, false) })
	if !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	if !strings.Contains(out, "I18N005") {
		t.Fatalf("--quiet hid the diagnostic:\n%s", out)
	}
}

func TestQuietSuppressesTheSuccessLine(t *testing.T) {
	path := project(t, valid)
	ctx := context.Background()

	loud, err := run(t, func() error { return cmdBuild(ctx, path, false) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loud, "generated") {
		t.Fatalf("no success line without --quiet:\n%s", loud)
	}

	path = project(t, valid)
	quiet = true
	defer func() { quiet = false }()

	hushed, err := run(t, func() error { return cmdBuild(ctx, path, false) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(hushed) != "" {
		t.Fatalf("--quiet still printed:\n%s", hushed)
	}
}
