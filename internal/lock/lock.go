// internal/lock/lock.go

// Package lock serialises runs that touch the same output directory.
//
// `loc watch` rebuilding while someone runs `loc build` by hand is two writers
// on one tree, and orphan detection reads a directory the other process is
// still writing to - so a file one run just produced can look like debris to
// the other and be deleted.
package lock

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Lock is held for the life of an operation. The zero value is safe to Release.
type Lock struct {
	file *os.File
}

// Acquire takes an advisory lock covering one output directory. Exclusive for
// anything that writes, shared for `check`, so concurrent checks do not queue.
//
// It blocks rather than failing when another run holds the lock, but says so
// first: a tool that appears to hang is worse than one that waits out loud.
func Acquire(output string, exclusive bool) (*Lock, error) {
	path, err := pathFor(output)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: open %s: %w", path, err)
	}

	err = acquire(file, exclusive, false)
	if errors.Is(err, errWouldBlock) {
		fmt.Fprintln(os.Stderr, "loc: waiting for another loc run to finish")
		err = acquire(file, exclusive, true)
	}
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("lock: %s: %w", path, err)
	}
	return &Lock{file: file}, nil
}

func (l *Lock) Release() {
	if l == nil || l.file == nil {
		return
	}
	_ = release(l.file)
	_ = l.file.Close()
	l.file = nil
}

// pathFor names the lock after the output directory it covers.
//
// In TempDir, not in the output directory: `check` needs a lock and must not
// create that directory, which is a guarantee its own test asserts. Not in the
// config directory either, since that is source-controlled and a stray lock
// file would show up in every consuming repo's `git status`.
//
// Keyed on the output rather than the config so two configs writing to one
// output still serialise, which is the invariant that matters.
func pathFor(output string) (string, error) {
	abs, err := filepath.Abs(output)
	if err != nil {
		return "", fmt.Errorf("lock: resolve %s: %w", output, err)
	}

	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	return filepath.Join(os.TempDir(), "loc-"+hex.EncodeToString(sum[:])[:16]+".lock"), nil
}
