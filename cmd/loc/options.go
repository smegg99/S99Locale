// cmd/loc/options.go

package main

import (
	"github.com/charmbracelet/colorprofile"

	"github.com/smegg99/s99locale/internal/config"
)

// options holds the persistent flags and --strict; other flags stay local to their command.
type options struct {
	configPath string
	// strict is registered on build, check, watch and lint only.
	strict bool
	quiet  bool
	// profile is what --color resolved to; zero means detect.
	profile colorprofile.Profile
}

func defaultOptions() options { return options{configPath: config.Default} }
