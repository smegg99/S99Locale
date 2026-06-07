// internal/config/load.go

package config

import (
	"fmt"
	"os"

	"cuelang.org/go/cue"
)

// Default is where loc looks when no path is given.
const Default = "i18n/config.cue"

// loadFile compiles a config file on its own, before unification, so a syntax
// error names the user's file rather than the embedded schema.
func loadFile(ctx *cue.Context, path string) (cue.Value, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return cue.Value{}, fmt.Errorf("config: read %s: %w", path, err)
	}

	v := ctx.CompileBytes(raw, cue.Filename(path))
	if err := v.Err(); err != nil {
		return cue.Value{}, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return v, nil
}
