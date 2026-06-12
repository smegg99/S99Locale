// internal/scan/scan_test.go

package scan_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/smegg99/s99locale/internal/scan"
)

func tree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func run(t *testing.T, files map[string]string, opts ...func(*scan.Options)) *scan.Result {
	t.Helper()
	o := scan.Options{Roots: []string{tree(t, files)}}
	for _, fn := range opts {
		fn(&o)
	}

	res, err := scan.Run(o)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestFindsVueI18nCalls(t *testing.T) {
	res := run(t, map[string]string{
		"app/pages/a.vue": `
<template>
  <div>{{ t('auth.login.title') }}</div>
  <span>{{ $t("common.cancel") }}</span>
</template>
<script setup lang="ts">
const label = t('common.close')
if (te('maybe.missing')) {}
</script>
`,
	})

	want := []string{"auth.login.title", "common.cancel", "common.close", "maybe.missing"}
	if got := res.Keys(); !slices.Equal(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
}

func TestRecordsFileAndLine(t *testing.T) {
	res := run(t, map[string]string{
		"a.ts": "const x = 1\nconst y = t('common.cancel')\n",
	})

	refs := res.Refs["common.cancel"]
	if len(refs) != 1 {
		t.Fatalf("refs = %v", refs)
	}
	if refs[0].Line != 2 {
		t.Fatalf("line = %d, want 2", refs[0].Line)
	}
	if filepath.Base(refs[0].File) != "a.ts" {
		t.Fatalf("file = %s", refs[0].File)
	}
}

// A key-shaped string is required, or every t('some sentence') becomes a key.
func TestIgnoresNonKeyArguments(t *testing.T) {
	res := run(t, map[string]string{
		"a.ts": "t('hello world')\nt('')\nt(variable)\n",
	})

	if len(res.Keys()) != 0 {
		t.Fatalf("keys = %v, want none", res.Keys())
	}
}

// An interpolated template is a dynamic key and is not matched.
func TestInterpolatedTemplateIsNotAKey(t *testing.T) {
	body := "const a = t(`" + "status.${state}" + "`)\n" +
		"const b = t(`" + "common.cancel" + "`)\n"

	res := run(t, map[string]string{"a.ts": body})

	// The plain template literal is found; the interpolated one is not.
	if got := res.Keys(); !slices.Equal(got, []string{"common.cancel"}) {
		t.Fatalf("keys = %v, want only common.cancel", got)
	}
}

// Generated Go constants and accessors map back to their keys.
func TestFindsGoAccessorsAndKeyConstants(t *testing.T) {
	names := map[string]string{
		"ReportHeading": "report.heading",
		"ReportSummary": "report.summary",
	}

	res := run(t, map[string]string{
		"main.go": `
package main

func render(l *i18n.Localizer) {
	_ = messages.ReportHeading(l)
	_ = messages.KeyReportSummary
}
`,
	}, func(o *scan.Options) { o.GoNames = names })

	want := []string{"report.heading", "report.summary"}
	if got := res.Keys(); !slices.Equal(got, want) {
		t.Fatalf("keys = %v, want %v", got, want)
	}
}

// A Go identifier that is not a generated accessor must not become a key.
func TestUnknownGoIdentifiersAreIgnored(t *testing.T) {
	res := run(t, map[string]string{
		"main.go": "package main\n\nfunc f() { _ = SomethingElse(ctx) }\n",
	}, func(o *scan.Options) { o.GoNames = map[string]string{"Known": "a.b"} })

	if len(res.Keys()) != 0 {
		t.Fatalf("keys = %v, want none", res.Keys())
	}
}

// The generated bundles name every key, so they are not scanned.
func TestGeneratedDirectoryIsSkipped(t *testing.T) {
	dir := tree(t, map[string]string{
		"app/a.ts":            "t('real.key')\n",
		"generated/keys.ts":   "t('generated.key')\n",
		"node_modules/x/a.ts": "t('vendored.key')\n",
	})

	res, err := scan.Run(scan.Options{
		Roots:     []string{dir},
		Generated: filepath.Join(dir, "generated"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := res.Keys(); !slices.Equal(got, []string{"real.key"}) {
		t.Fatalf("keys = %v, want only real.key", got)
	}
}

func TestMissingRootIsNotAnError(t *testing.T) {
	res, err := scan.Run(scan.Options{Roots: []string{"/does/not/exist"}})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(res.Keys()) != 0 {
		t.Fatalf("keys = %v", res.Keys())
	}
}

func TestUnusedReportsWhatNothingReferences(t *testing.T) {
	res := run(t, map[string]string{"a.ts": "t('used.key')\n"})

	declared := []string{"used.key", "dead.key", "also.dead"}
	got := scan.Unused(declared, res, nil)

	if !slices.Equal(got, []string{"also.dead", "dead.key"}) {
		t.Fatalf("unused = %v", got)
	}
}

// A declared dynamic key stays out of the unused report.
func TestDynamicKeysAreNotUnused(t *testing.T) {
	res := run(t, map[string]string{"a.ts": "t('used.key')\n"})

	declared := []string{"used.key", "status.pending", "status.done"}
	got := scan.Unused(declared, res, []string{"status.pending", "status.done"})

	if len(got) != 0 {
		t.Fatalf("unused = %v, want none", got)
	}
}

// A trailing dot covers every key beneath it.
func TestDynamicPrefixCoversItsChildren(t *testing.T) {
	res := run(t, map[string]string{"a.ts": "t('used.key')\n"})

	declared := []string{"used.key", "status.pending", "status.failed", "other.dead"}
	got := scan.Unused(declared, res, []string{"status."})

	if !slices.Equal(got, []string{"other.dead"}) {
		t.Fatalf("unused = %v, want only other.dead", got)
	}
}

// A reference to an undeclared key is reported.
func TestMissingReportsReferencesWithNoKey(t *testing.T) {
	res := run(t, map[string]string{"a.ts": "t('real.key')\nt('typo.kye')\n"})

	got := scan.Missing([]string{"real.key"}, res)
	if len(got) != 1 {
		t.Fatalf("missing = %v", got)
	}
	if got[0].Key != "typo.kye" {
		t.Fatalf("key = %q", got[0].Key)
	}
	// With a location.
	if got[0].Line == 0 || got[0].File == "" {
		t.Fatalf("no location: %+v", got[0])
	}
}

func TestCountsFilesScanned(t *testing.T) {
	res := run(t, map[string]string{
		"a.ts":      "t('a.b')\n",
		"b.vue":     "t('c.d')\n",
		"c.go":      "package main\n",
		"readme.md": "t('not.scanned')\n",
		"d.unknown": "t('nope')\n",
	})

	// Only source extensions count.
	if res.Files != 3 {
		t.Fatalf("files = %d, want 3", res.Files)
	}
	if slices.Contains(res.Keys(), "not.scanned") {
		t.Fatal("a markdown file was scanned")
	}
}

func TestSameKeyInSeveralPlaces(t *testing.T) {
	res := run(t, map[string]string{
		"a.ts": "t('common.cancel')\n",
		"b.ts": "t('common.cancel')\nt('common.cancel')\n",
	})

	if n := len(res.Refs["common.cancel"]); n != 3 {
		t.Fatalf("%d references, want 3", n)
	}
}

// Accessors are found whatever the localizer variable is called.
func TestGoAccessorFoundWhateverTheArgumentIsCalled(t *testing.T) {
	names := map[string]string{"ReportSummary": "report.summary"}

	for _, call := range []string{
		"messages.ReportSummary(l, p)",
		"messages.ReportSummary(en, p)",
		"messages.ReportSummary(polish, messages.ReportSummaryParams{})",
		"_ = messages.ReportSummary( localizer , p )",
	} {
		res := run(t, map[string]string{
			"main.go": "package main\n\nfunc f() { " + call + " }\n",
		}, func(o *scan.Options) { o.GoNames = names })

		if len(res.Refs["report.summary"]) == 0 {
			t.Errorf("not found in: %s", call)
		}
	}
}

// The apierr envelope's message key is scanned.
func TestFindsApierrMessageKeys(t *testing.T) {
	res := run(t, map[string]string{
		"h.go": `
package handlers

func f() {
	apierr.Respond(c, apierr.New(apierr.NotFound, "item.notFound"))
	apierr.Respond(c, apierr.Wrap(err, apierr.Validation, "share.invalid"))
	return apierr.Wrap(err, apierr.Internal, "file.failed")
}
`,
	})

	for _, want := range []string{"item.notFound", "share.invalid", "file.failed"} {
		if len(res.Refs[want]) == 0 {
			t.Errorf("%s not found; got %v", want, res.Keys())
		}
	}
}

// A kind is not a message key: apierr.NotFound is an identifier, not a string.
func TestApierrKindsAreNotKeys(t *testing.T) {
	res := run(t, map[string]string{
		"h.go": "package h\n\nfunc f() { apierr.Respond(c, apierr.New(apierr.NotFound, \"a.b\")) }\n",
	})

	if got := res.Keys(); len(got) != 1 || got[0] != "a.b" {
		t.Fatalf("keys = %v, want only a.b", got)
	}
}

// Config patterns find keys passed through a repo's own helpers.
func TestExtraPatterns(t *testing.T) {
	extra, err := scan.Compile([]string{`mapErr\([^,]+,\s*"([\w.]+)"`})
	if err != nil {
		t.Fatal(err)
	}

	res := run(t, map[string]string{
		"h.go": "package h\n\nfunc f() { return mapErr(err, \"item.createFailed\") }\n",
	}, func(o *scan.Options) { o.Extra = extra })

	if len(res.Refs["item.createFailed"]) == 0 {
		t.Fatalf("keys = %v", res.Keys())
	}
}

// A pattern with the wrong number of groups is rejected at load.
func TestPatternMustHaveExactlyOneGroup(t *testing.T) {
	for _, bad := range []string{`no groups here`, `(two)(groups)`} {
		if _, err := scan.Compile([]string{bad}); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	if _, err := scan.Compile([]string{`([`}); err == nil {
		t.Error("accepted an invalid regexp")
	}
}

// A copy of the generated bundle outside the output directory is skipped too.
func TestACopyOfTheGeneratedOutputDoesNotGradeItself(t *testing.T) {
	res := run(t, map[string]string{
		"generated/go/keys_gen.go":      "// Code generated by loc. DO NOT EDIT.\n\npackage messages\n\nconst KeyTrayOpen = \"tray.open\"\n",
		"internal/messages/keys_gen.go": "// Code generated by loc. DO NOT EDIT.\n\npackage messages\n\nconst KeyTrayOpen = \"tray.open\"\n",
		"main.go":                       "package main\n\nfunc f() { _ = 1 }\n",
	}, func(o *scan.Options) {
		o.GoNames = map[string]string{"TrayOpen": "tray.open"}
	})

	if len(res.Refs["tray.open"]) != 0 {
		t.Fatalf("a generated copy counted as a reference: %v", res.Refs["tray.open"])
	}
}
