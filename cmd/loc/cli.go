// cmd/loc/cli.go

package main

import (
	"errors"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale"
	"github.com/smegg99/s99locale/internal/catalog"
	"github.com/smegg99/s99locale/internal/config"
)

// errFailed means a command printed its own diagnostics; exit non-zero without another message.
var errFailed = errors.New("failed")

func rootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:           "loc",
		Version:       s99locale.Version(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("loc {{.Version}}\n")
	// Errors are human lines. Help, version and completion stay on stdout, where a shell reads them.
	root.SetErr(a.out)

	root.PersistentFlags().StringVarP(&a.opts.configPath, "config", "c", config.Default, "")
	root.PersistentFlags().BoolVar(&a.opts.quiet, "quiet", false, "")
	// --lang must exist before any describer looks it up.
	a.registerLang(root)

	root.AddCommand(
		newBuildCmd(a),
		newCheckCmd(a),
		newStatsCmd(a),
		newLintCmd(a),
		newWatchCmd(a),
		newCleanCmd(a),
	)

	a.describe(func(l *i18n.Localizer) {
		root.Short = catalog.CliRootShort(l)
		root.Long = catalog.CliRootLong(l)
		root.PersistentFlags().Lookup("config").Usage = catalog.CliFlagConfig(l)
		root.PersistentFlags().Lookup("quiet").Usage = catalog.CliFlagQuiet(l)
		root.PersistentFlags().Lookup("lang").Usage = catalog.CliFlagLang(l)
	})

	// After AddCommand, so help and completion exist before the describe pass.
	initDefaults(a, root)
	// The only place the environment is read.
	a.localize(catalog.FromEnv())

	return root
}
