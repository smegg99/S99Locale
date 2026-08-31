// cmd/loc/locale.go

package main

import (
	"errors"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale/internal/catalog"
)

// registerLang adds --lang; its callback fires in ParseFlags, before cobra reads help and version.
func (a *app) registerLang(root *cobra.Command) {
	root.PersistentFlags().Func("lang", "", func(value string) error {
		match, ok := catalog.Supported(value)
		if !ok {
			return errors.New(catalog.CliErrUnsupportedLanguage(a.say(), catalog.CliErrUnsupportedLanguageParams{
				Lang:      value,
				Supported: strings.Join(catalog.Locales, ", "),
			}))
		}
		a.localize(match)
		return nil
	})
}

// registerColor adds --color; its callback fires in ParseFlags, before anything is printed.
func (a *app) registerColor(root *cobra.Command) {
	root.PersistentFlags().Func("color", "", func(value string) error {
		switch value {
		case "auto":
			// Zero means detect, as in s99term.New.
			a.opts.profile = 0
		case "always":
			a.opts.profile = colorprofile.TrueColor
		case "never":
			a.opts.profile = colorprofile.NoTTY
		default:
			return errors.New(catalog.CliErrUnknownColor(a.say(), catalog.CliErrUnknownColorParams{
				Value: value,
			}))
		}
		a.invalidate()
		return nil
	})
}

// noArgs answers a stray positional in the reader's language, which cobra.NoArgs does not.
func (a *app) noArgs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return errors.New(catalog.CliErrNoArgs(a.say(), catalog.CliErrNoArgsParams{
			Command: cmd.CommandPath(),
		}))
	}
	return nil
}

// initDefaults registers the flags and commands cobra adds during Execute, and describes them.
func initDefaults(a *app, root *cobra.Command) {
	// Registered here because Command.Find strips flags before execution: an unregistered --help
	// takes a value, which swallows --lang and leaves its value as a positional.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()

	var visit func(*cobra.Command)
	visit = func(c *cobra.Command) {
		c.InitDefaultHelpFlag()
		c.InitDefaultVersionFlag()
		for _, child := range c.Commands() {
			visit(child)
		}
	}
	visit(root)

	a.describe(func(l *i18n.Localizer) {
		var describe func(*cobra.Command)
		describe = func(c *cobra.Command) {
			if flag := c.Flags().Lookup("help"); flag != nil {
				flag.Usage = catalog.CliFlagHelp(l, catalog.CliFlagHelpParams{Command: c.Name()})
			}
			if flag := c.Flags().Lookup("version"); flag != nil {
				flag.Usage = catalog.CliFlagVersion(l)
			}
			for _, child := range c.Commands() {
				describe(child)
			}
		}
		describe(root)

		for _, c := range root.Commands() {
			switch c.Name() {
			case "help":
				c.Short = catalog.CliHelpShort(l)
				c.Long = catalog.CliHelpLong(l)
			case "completion":
				// Its own two descriptions only: the shell children's install recipes stay in English.
				c.Short = catalog.CliCompletionShort(l)
				c.Long = catalog.CliCompletionLong(l)
			}
		}

		root.SetUsageTemplate(usageTemplate(l))
	})
}

// cobraUsageTemplate is cobra v1.10.2's defaultUsageTemplate verbatim, so upgrades show a diff.
const cobraUsageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// usageTemplate replaces cobra's headings, which live in the template rather than in any Short field.
func usageTemplate(l *i18n.Localizer) string {
	// Ordered, not a map: "Flags:" is a substring of "Global Flags:".
	replacements := []struct{ from, to string }{
		{"Usage:", catalog.CliHeadingUsage(l)},
		{"Aliases:", catalog.CliHeadingAliases(l)},
		{"Examples:", catalog.CliHeadingExamples(l)},
		{"Available Commands:", catalog.CliHeadingCommands(l)},
		{"Additional Commands:", catalog.CliHeadingAdditionalCommands(l)},
		{"Global Flags:", catalog.CliHeadingGlobalFlags(l)},
		{"Flags:", catalog.CliHeadingFlags(l)},
		{"Additional help topics:", catalog.CliHeadingHelpTopics(l)},
		{
			`Use "{{.CommandPath}} [command] --help" for more information about a command.`,
			catalog.CliHeadingMoreInfo(l, catalog.CliHeadingMoreInfoParams{Command: "{{.CommandPath}}"}),
		},
	}

	out := cobraUsageTemplate
	for _, r := range replacements {
		out = strings.Replace(out, r.from, r.to, 1)
	}
	return out
}
