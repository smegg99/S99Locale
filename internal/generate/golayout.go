// internal/generate/golayout.go

package generate

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/smegg99/s99locale/internal/config"
	"github.com/smegg99/s99locale/internal/message"
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

// Owns reports whether loc may remove rel from a directory it shares with
// hand-written source.
func Owns(dir, rel string) (bool, error) {
	rel = path.Clean(rel)
	if strings.HasPrefix(rel, "locales/") {
		return true, nil
	}
	if strings.Contains(rel, "/") || !strings.HasSuffix(rel, "_gen.go") {
		return false, nil
	}
	// A *_gen.go must carry loc's own header. Another generator's output has
	// the same name shape: S99Deploy holds a cue_types_main_gen.go written by
	// `cue exp gengotypes`, and deleting it would be silent and unrecoverable.
	return hasGeneratedHeader(filepath.Join(dir, filepath.FromSlash(rel)))
}

// hasGeneratedHeader reads the first line only. A file that is gone is not
// loc's to remove either, which is why IsNotExist is false and not an error.
func hasGeneratedHeader(full string) (bool, error) {
	file, err := os.Open(full)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("generate: open %s: %w", full, err)
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	first, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("generate: read %s: %w", full, err)
	}
	return strings.HasPrefix(first, message.GeneratedHeader), nil
}
