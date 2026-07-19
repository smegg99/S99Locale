// cmd/loc/cli.go

package main

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/smegg99/s99locale"
	"github.com/smegg99/s99locale/internal/config"
)

// errFailed means a command printed its own diagnostics; exit non-zero without another message.
var errFailed = errors.New("failed")

func rootCmd(a *app) *cobra.Command {
	root := &cobra.Command{
		Use:   "loc",
		Short: "Compile one canonical locale source into per-target bundles",
		Long: "loc compiles canonical YAML into vue-i18n and go-i18n bundles,\n" +
			"typed keys and a manifest. Generated output is committed.",
		Version:       s99locale.Version(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("loc {{.Version}}\n")
	// Errors are human lines. Help, version and completion stay on stdout, where a shell reads them.
	root.SetErr(a.out)

	root.PersistentFlags().StringVarP(&a.opts.configPath, "config", "c", config.Default, "config file")
	root.PersistentFlags().BoolVar(&a.opts.quiet, "quiet", false, "suppress success output; diagnostics are unaffected")

	root.AddCommand(
		newBuildCmd(a),
		newCheckCmd(a),
		newStatsCmd(a),
		newLintCmd(a),
		newWatchCmd(a),
		newCleanCmd(a),
	)
	return root
}
