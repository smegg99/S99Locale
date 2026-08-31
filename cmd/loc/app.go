// cmd/loc/app.go

package main

import (
	"io"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/smegg99/s99term"

	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/validate"
)

// app is the command tree's shared state: output streams and parsed flags.
type app struct {
	// out carries human lines, written only through the console.
	out io.Writer
	// data carries --json output only.
	data io.Writer
	// env feeds terminal capability detection.
	env      []string
	opts     options
	words    *i18n.Localizer
	language string
	// describes re-apply the language to the command tree.
	describes []func(*i18n.Localizer)
	term      *s99term.Console
}

func newApp(out, data io.Writer, env []string) *app {
	return &app{out: out, data: data, env: env, opts: defaultOptions()}
}

// screen is built on first use, so --lang and --color can change it during flag parsing.
func (a *app) screen() *s99term.Console {
	if a.term == nil {
		a.term = s99term.New(s99term.Options{
			Out:     a.out,
			Env:     a.env,
			Words:   a.statusWords(),
			Profile: a.opts.profile,
		})
	}
	return a.term
}

// invalidate drops the console so the next line uses the flags just parsed.
func (a *app) invalidate() {
	if a.term != nil {
		a.term.Close()
		a.term = nil
	}
}

// close restores the cursor; every run defers it.
func (a *app) close() {
	if a.term != nil {
		a.term.Close()
	}
}

// statusWords are s99term's state words, taken from loc's catalogue.
func (a *app) statusWords() s99term.Words {
	l := a.say()
	return s99term.Words{
		s99term.StatusOK:      catalog.CliStatusOk(l),
		s99term.StatusFailed:  catalog.CliStatusFailed(l),
		s99term.StatusSkipped: catalog.CliStatusSkipped(l),
		s99term.StatusWaiting: catalog.CliStatusWaiting(l),
		s99term.StatusRunning: catalog.CliStatusRunning(l),
		s99term.StatusWarning: catalog.CliStatusWarning(l),
		s99term.StatusStale:   catalog.CliStatusStale(l),
	}
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
	// The console holds state words in the previous language, so it is rebuilt.
	a.invalidate()
}

// say is the localizer in use, the default locale's until another is picked.
func (a *app) say() *i18n.Localizer {
	if a.words == nil {
		a.words = catalog.Localizer(catalog.DefaultLocale)
	}
	return a.words
}

// says prints a line that says a run went fine, unless --quiet.
func (a *app) says(st s99term.Status, label string, fields ...s99term.Field) {
	if a.opts.quiet {
		return
	}
	a.screen().Line(st, label, fields...)
}

// warns prints a line that survives --quiet, so a quiet run that fails still says why.
func (a *app) warns(st s99term.Status, label string, fields ...s99term.Field) {
	a.screen().Line(st, label, fields...)
}

// block prints an already-composed body, such as a rendered diagnostic or a table.
func (a *app) block(body string) { a.screen().Print(body) }

// begin starts a live step, unless --quiet: a spinner is success output too.
func (a *app) begin(label string) *s99term.Step {
	if a.opts.quiet {
		return nil
	}
	return a.screen().Begin(label)
}

// end finishes a step begin may not have started.
func (a *app) end(step *s99term.Step, st s99term.Status, fields ...s99term.Field) {
	if step == nil {
		return
	}
	step.End(st, fields...)
}

// printReport ends each diagnostic in a blank line, separating it from the next and the totals.
func (a *app) printReport(r *validate.Report) {
	for _, d := range r.Diagnostics {
		a.block(a.renderDiagnostic(d) + "\n")
	}

	errs, warns := len(r.Errors()), len(r.Warnings())
	if errs+warns == 0 {
		return
	}
	state := s99term.StatusWarning
	if errs > 0 {
		state = s99term.StatusFailed
	}
	a.warns(state, catalog.ReportErrorsAndWarnings(a.say(), catalog.ReportErrorsAndWarningsParams{
		Errors: errs, Warnings: warns}))
}

// waitingForLock says a run is waiting for another; it survives --quiet so the wait is visible.
func (a *app) waitingForLock() {
	a.warns(s99term.StatusWaiting, catalog.ReportWaitingForLock(a.say()))
}

// table is loc's one table shape: the border follows the charset, the widths follow the content.
func (a *app) table(headers []string, rows [][]string) string {
	return a.screen().Styles().Table().Headers(headers...).Rows(rows...).Render()
}

// pairs is a borderless label and value table, so labels line up without a hand-set width.
func (a *app) pairs(rows [][2]string) string {
	built := a.screen().Styles().Table().Border(lipgloss.HiddenBorder()).BorderTop(false).
		BorderBottom(false).BorderLeft(false).BorderRight(false).BorderColumn(false)
	for _, row := range rows {
		label := row[0]
		if label != "" {
			label += ":"
		}
		built = built.Row(label, row[1])
	}
	return trimRows(built.Render())
}

// trimRows drops trailing cell padding, so a value is the last thing on its line.
func trimRows(block string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return strings.Join(lines, "\n")
}
