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

func rootCmd() *cobra.Command {
	var (
		configPath string
		strict     bool
	)

	root := &cobra.Command{
		Use:   "loc",
		Short: "Compile one canonical locale source into per-target bundles",
		Long: "loc compiles canonical YAML into vue-i18n and go-i18n bundles,\n" +
			"typed keys and a manifest. Generated output is disposable.",
		Version:       s99locale.Version(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("loc {{.Version}}\n")

	root.PersistentFlags().StringVarP(&configPath, "config", "c", config.Default, "config file")
	root.PersistentFlags().BoolVar(&strict, "strict", false, "warnings fail the build")
	root.PersistentFlags().BoolVar(&quiet, "quiet", false, "suppress success output; diagnostics are unaffected")

	root.AddCommand(
		&cobra.Command{
			Use:   "build",
			Short: "Validate and generate every configured target",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return cmdBuild(cmd.Context(), configPath, strict)
			},
		},
		checkCmd(&configPath, &strict),
		&cobra.Command{
			Use:   "stats",
			Short: "Counts by namespace, target and locale, plus bundle sizes",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return cmdStats(cmd.Context(), configPath)
			},
		},
		lintCmd(&configPath),
		&cobra.Command{
			Use:   "watch",
			Short: "Rebuild on every change until interrupted",
			Long: "Survives a validation error and keeps watching: a half-typed key is\n" +
				"the normal state of a file being edited.",
			Args: cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return cmdWatch(cmd.Context(), configPath, strict)
			},
		},
		&cobra.Command{
			Use:   "clean",
			Short: "Remove generated artefacts",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				return cmdClean(configPath)
			},
		},
	)
	return root
}

func checkCmd(configPath *string, strict *bool) *cobra.Command {
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate and confirm the generated output is current",
		Long: "Never writes, which is what CI runs. Exit 1 means the locales are\n" +
			"wrong or the output is stale; exit 2 means loc could not run at all.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdCheck(cmd.Context(), *configPath, *strict, asJSON)
		},
	}

	cmd.Flags().BoolVar(&asJSON, "json", false, "machine-readable output")

	return cmd
}

func lintCmd(configPath *string) *cobra.Command {
	var (
		verbose bool
		asJSON  bool
	)

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Report messages that may be duplicates",
		Long: "Never modifies anything. Merging is a decision, and an ordinary\n" +
			"build is not the place to make it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmdLintShared(cmd.Context(), *configPath, verbose, asJSON)
		},
	}

	cmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "include medium and low confidence findings")
	cmd.PersistentFlags().BoolVar(&asJSON, "json", false, "machine-readable output")

	cmd.AddCommand(
		&cobra.Command{
			Use:   "shared",
			Short: "Keys that may belong in a shared namespace",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return cmdLintShared(cmd.Context(), *configPath, verbose, asJSON)
			},
		},
		&cobra.Command{
			Use:   "keys",
			Short: "Unused and missing keys, from a source scan",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return cmdLintKeys(cmd.Context(), *configPath, asJSON)
			},
		},
		&cobra.Command{
			Use:   "duplicates",
			Short: "Exact duplicate contracts, whatever they are named",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return cmdLintDuplicates(cmd.Context(), *configPath, asJSON)
			},
		},
	)
	return cmd
}
