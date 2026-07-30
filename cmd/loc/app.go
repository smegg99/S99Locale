// cmd/loc/app.go

package main

import (
	"fmt"
	"io"

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
}

func newApp(out, data io.Writer) *app {
	return &app{out: out, data: data, opts: defaultOptions()}
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

func (a *app) printReport(r *validate.Report) {
	for _, d := range r.Diagnostics {
		a.warns("%s\n", d.String())
	}

	errs, warns := len(r.Errors()), len(r.Warnings())
	if errs+warns > 0 {
		a.warns("%d error(s), %d warning(s)\n", errs, warns)
	}
}

// waitingForLock says a run is waiting for another; it survives --quiet so the wait is visible.
func (a *app) waitingForLock() {
	a.warns("loc: waiting for another loc run to finish\n")
}
