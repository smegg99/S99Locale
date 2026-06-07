// internal/generate/fs.go

package generate

import (
	"fmt"
	"os"
	"path/filepath"
)

// tempPrefix marks a file that a build is still staging. Nothing outside a
// build's own commit phase may treat one as output - the orphan sweep skips
// them, or a concurrent run's sweep would delete a file mid-write.
const tempPrefix = ".loc-"

// fileSystem is what Write commits through, so a test can fail a write on
// demand. Reads stay on os directly: the read path has no failure worth
// injecting.
type fileSystem interface {
	MkdirAll(path string) error
	// WriteTemp writes body beside its destination with the final mode and
	// returns the temp path, so Rename is the only step left to visibility.
	WriteTemp(dir string, body []byte) (string, error)
	Rename(from, to string) error
	Remove(path string) error
}

type osFS struct{}

func (osFS) MkdirAll(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("generate: mkdir %s: %w", path, err)
	}
	return nil
}

func (osFS) WriteTemp(dir string, body []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("generate: temp in %s: %w", dir, err)
	}
	name := tmp.Name()

	if _, err := tmp.Write(body); err != nil {
		tmp.Close()
		os.Remove(name)
		return "", fmt.Errorf("generate: write %s: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("generate: close %s: %w", name, err)
	}
	if err := os.Chmod(name, 0o644); err != nil {
		os.Remove(name)
		return "", fmt.Errorf("generate: chmod %s: %w", name, err)
	}
	return name, nil
}

func (osFS) Rename(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("generate: rename into %s: %w", to, err)
	}
	return nil
}

func (osFS) Remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("generate: remove %s: %w", filepath.Base(path), err)
	}
	return nil
}
