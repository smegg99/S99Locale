// internal/generate/clean.go

package generate

import "github.com/smegg99/s99locale/internal/config"

// Clean removes recorded files loc can prove it wrote, then the records and emptied directories.
func Clean(cfg *config.Config) (removed []string, err error) {
	// Writing empty trees runs the build's sweep; an unproven file stops it before any removal.
	// A tree with no record cleans nothing.
	outs, err := trees(cfg)
	if err != nil {
		return nil, err
	}
	if _, removed, err = outs.Write(); err != nil {
		return removed, err
	}
	// The default parent is empty once every tree under it is gone.
	return removed, removeIfEmpty(cfg.AbsOutput())
}
