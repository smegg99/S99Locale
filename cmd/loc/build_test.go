// cmd/loc/build_test.go

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

// --quiet hides only success lines; a quiet run that fails still says why.
func TestQuietKeepsDiagnosticsAndFailure(t *testing.T) {
	path := project(t, map[string]string{
		"en/common.yaml": "cancel:\n  value: \"Cancel\"\n",
		"pl/common.yaml": "",
	})

	a, out, _ := testApp(t, options{configPath: path, quiet: true})
	if err := cmdBuild(context.Background(), a); !errors.Is(err, errFailed) {
		t.Fatalf("err = %v, want a failure", err)
	}
	if !strings.Contains(out.String(), "I18N005") {
		t.Fatalf("--quiet hid the diagnostic:\n%s", out.String())
	}
}

func TestQuietSuppressesTheSuccessLine(t *testing.T) {
	loudApp, loud, _ := testApp(t, options{configPath: project(t, valid)})
	if err := cmdBuild(context.Background(), loudApp); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(loud.String(), "generated") {
		t.Fatalf("no success line without --quiet:\n%s", loud.String())
	}

	hushedApp, hushed, _ := testApp(t, options{configPath: project(t, valid), quiet: true})
	if err := cmdBuild(context.Background(), hushedApp); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(hushed.String()) != "" {
		t.Fatalf("--quiet still printed:\n%s", hushed.String())
	}
}

// Every human line goes to the human stream, so stdout carries only machine output.
func TestBuildWritesNothingToTheDataStream(t *testing.T) {
	a, _, data := testApp(t, options{configPath: project(t, valid)})

	if err := cmdBuild(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if data.Len() != 0 {
		t.Fatalf("build wrote to stdout:\n%s", data.String())
	}
}

// --strict is registered only on the commands that read it.
func TestStrictIsOnlyOnTheCommandsThatUseIt(t *testing.T) {
	root := rootCmd(&app{})

	want := map[string]bool{
		"build": true, "check": true, "watch": true, "lint": true,
		"stats": false, "clean": false,
	}
	if root.PersistentFlags().Lookup("strict") != nil {
		t.Fatal("--strict is still persistent on the root")
	}
	for name, has := range want {
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd.Name() != name {
			t.Errorf("%s is not registered: %v", name, err)
			continue
		}
		got := cmd.Flags().Lookup("strict") != nil || cmd.PersistentFlags().Lookup("strict") != nil
		if got != has {
			t.Errorf("%s has --strict = %v, want %v", cmd.Name(), got, has)
		}
	}
}

// Each registered flag has to reach the options the command reads.
func TestFlagsReachTheOptions(t *testing.T) {
	for _, path := range [][]string{{"build"}, {"check"}, {"watch"}, {"lint"}, {"lint", "keys"}} {
		a := newApp(io.Discard, io.Discard)
		cmd, _, err := rootCmd(a).Find(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.ParseFlags([]string{"--strict"}); err != nil {
			t.Fatal(err)
		}
		if !a.opts.strict {
			t.Errorf("%s --strict did not set options.strict", cmd.CommandPath())
		}
	}

	a := newApp(io.Discard, io.Discard)
	if err := rootCmd(a).ParseFlags([]string{"-c", "elsewhere.cue", "--quiet"}); err != nil {
		t.Fatal(err)
	}
	if a.opts.configPath != "elsewhere.cue" || !a.opts.quiet {
		t.Errorf("root flags did not reach the options: %+v", a.opts)
	}
}

// The same flags through Execute: parsed by Cobra, read by the running command.
func TestBuildRunsWithItsFlagsThroughTheTree(t *testing.T) {
	out := &bytes.Buffer{}
	root := rootCmd(newApp(out, &bytes.Buffer{}))
	root.SetArgs([]string{"build", "--strict", "--quiet", "-c", project(t, onlyWarnings)})
	if err := root.Execute(); !errors.Is(err, errFailed) {
		t.Fatalf("build --strict on a warning = %v, want errFailed\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "warning(s)") {
		t.Fatalf("--quiet hid the warning:\n%s", out.String())
	}

	hushed := &bytes.Buffer{}
	root = rootCmd(newApp(hushed, &bytes.Buffer{}))
	root.SetArgs([]string{"--quiet", "build", "-c", project(t, valid)})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if hushed.Len() != 0 {
		t.Fatalf("--quiet still printed:\n%s", hushed.String())
	}
}
