// internal/generate/golayout.go

package generate

import (
	"fmt"
	"path"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
)

// layout is where the Go target's files go, relative to the output they are
// staged in.
//
// Two shapes, because go:embed cannot reach outside its own package. Under the
// output directory the Go target sits beside the others, in `go/` and
// `backend/`. Pointed at a consumer's package by `go.output`, the accessors sit
// beside that package's own source and the bundle goes in `locales/`, which is
// what the embed directive can name.
type layout struct{ shared bool }

func goLayout(cfg *config.Config) layout { return layout{shared: cfg.Go.Output != ""} }

func (l layout) messages() string {
	if l.shared {
		return "messages_gen.go"
	}
	return "go/messages_gen.go"
}

func (l layout) keys() string {
	if l.shared {
		return "keys_gen.go"
	}
	return "go/keys_gen.go"
}

func (l layout) bundle(locale string) string {
	if l.shared {
		return fmt.Sprintf("locales/%s.json", locale)
	}
	return fmt.Sprintf("backend/%s.json", locale)
}

// Owns is what loc may remove from a package it shares with hand-written
// source: the accessors it emits, and the whole bundle directory. `loc clean`
// asks the same question the build does.
func Owns(rel string) bool {
	rel = path.Clean(rel)
	if strings.HasPrefix(rel, "locales/") {
		return true
	}
	return !strings.Contains(rel, "/") && strings.HasSuffix(rel, "_gen.go")
}
