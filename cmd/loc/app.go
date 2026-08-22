// cmd/loc/app.go

package main

import (
	"fmt"
	"io"

	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/validate"
)

// app is the command tree's shared state: where its output goes and what the
// flags say.
type app struct {
	// out carries every human line. main sets it to stderr.
	out io.Writer
	// data carries machine-readable output only: --json. main sets it to stdout.
	data io.Writer
	opts options
	// words is the localizer every command renders through.
	words *i18n.Localizer
	// language is what words renders in, for anything that has to name it.
	language string
	// describes are the closures that re-apply the language to the tree.
	describes []func(*i18n.Localizer)
}

func newApp(out, data io.Writer) *app {
	return &app{out: out, data: data, opts: defaultOptions()}
}

// describe registers a command's own strings, so a command's text lives in the command's file.
func (a *app) describe(f func(*i18n.Localizer)) { a.describes = append(a.describes, f) }

// localize re-renders every registered description, at construction and when --lang parses.
func (a *app) localize(language string) {
	a.language = language
	a.words = catalog.Localizer(language)
	for _, f := range a.describes {
		f(a.words)
	}
}

// say is the localizer in use, the default locale's until another is picked.
func (a *app) say() *i18n.Localizer {
	if a.words == nil {
		a.words = catalog.Localizer(catalog.DefaultLocale)
	}
	return a.words
}

// says prints a line that says a run went fine, unless --quiet.
func (a *app) says(format string, args ...any) {
	if a.opts.quiet {
		return
	}
	fmt.Fprintf(a.out, format, args...)
}

// warns prints a line that survives --quiet, which is every line that does not say a run went fine.
func (a *app) warns(format string, args ...any) {
	fmt.Fprintf(a.out, format, args...)
}

// printReport ends each diagnostic in a blank line, separating it from the next and the totals.
func (a *app) printReport(r *validate.Report) {
	for _, d := range r.Diagnostics {
		a.warns("%s\n", a.renderDiagnostic(d))
	}

	errs, warns := len(r.Errors()), len(r.Warnings())
	if errs+warns > 0 {
		a.warns("%s\n", catalog.ReportErrorsAndWarnings(a.say(), catalog.ReportErrorsAndWarningsParams{
			Errors: errs, Warnings: warns}))
	}
}

// waitingForLock says a run is waiting for another; it survives --quiet so the wait is visible.
func (a *app) waitingForLock() {
	a.warns("loc: waiting for another loc run to finish\n")
}
