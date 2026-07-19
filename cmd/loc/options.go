// cmd/loc/options.go

package main

import "github.com/smegg99/s99locale/internal/config"

// options holds the persistent flags and --strict; other flags stay local to their command.
type options struct {
	configPath string
	// strict is registered on build, check, watch and lint only.
	strict bool
	// quiet is the persistent --quiet.
	quiet bool
}

func defaultOptions() options { return options{configPath: config.Default} }
