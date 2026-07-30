// internal/lock/lock.go

// Package lock serialises loc runs that touch one output directory.
package lock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// pollEvery is how often a waiting run retries flock.
const pollEvery = 50 * time.Millisecond

// Lock is one held advisory lock. The zero value is safe to release.
type Lock struct {
	file *os.File
}

// Held is a set of locks released together; nil and the zero value are safe to Release.
type Held struct {
	locks []*Lock
}

// AcquireAll takes one advisory lock per output directory: exclusive to write, shared for check.
func AcquireAll(ctx context.Context, dirs []string, exclusive bool, onWait func()) (*Held, error) {
	ordered, err := ordered(dirs)
	if err != nil {
		return nil, err
	}

	// onWait fires at most once across the whole set.
	said := false
	announce := func() {
		if said || onWait == nil {
			return
		}
		said = true
		onWait()
	}

	held := &Held{}
	for _, dir := range ordered {
		one, err := acquireOne(ctx, dir, exclusive, announce)
		if err != nil {
			held.Release()
			return nil, err
		}
		held.locks = append(held.locks, one)
	}
	return held, nil
}

// Release releases in reverse order.
func (h *Held) Release() {
	if h == nil {
		return
	}
	for i := len(h.locks) - 1; i >= 0; i-- {
		h.locks[i].release()
	}
	h.locks = nil
}

func (l *Lock) release() {
	if l == nil || l.file == nil {
		return
	}
	_ = release(l.file)
	_ = l.file.Close()
	l.file = nil
}

// ordered cleans, deduplicates and sorts the directories, so overlapping runs cannot deadlock.
func ordered(dirs []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(dirs))

	for _, dir := range dirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("lock: resolve %s: %w", dir, err)
		}
		abs = filepath.Clean(abs)
		if _, dup := seen[abs]; dup {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	sort.Strings(out)
	return out, nil
}

func acquireOne(ctx context.Context, dir string, exclusive bool, onWait func()) (*Lock, error) {
	path, err := pathFor(dir)
	if err != nil {
		return nil, err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: open %s: %w", path, err)
	}

	if err := wait(ctx, file, exclusive, onWait); err != nil {
		file.Close()
		return nil, fmt.Errorf("lock: %s: %w", path, err)
	}
	return &Lock{file: file}, nil
}

// wait takes the lock without ever blocking inside flock.
func wait(ctx context.Context, file *os.File, exclusive bool, onWait func()) error {
	err := acquire(file, exclusive)
	if !errors.Is(err, errWouldBlock) {
		return err
	}
	if onWait != nil {
		onWait()
	}

	// acquire never blocks; retrying on a ticker against ctx.Done() lets ctrl-c end a wait.
	tick := time.NewTicker(pollEvery)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
			err := acquire(file, exclusive)
			if errors.Is(err, errWouldBlock) {
				continue
			}
			return err
		}
	}
}

// pathFor names the lock after the output directory it covers.
func pathFor(output string) (string, error) {
	abs, err := filepath.Abs(output)
	if err != nil {
		return "", fmt.Errorf("lock: resolve %s: %w", output, err)
	}

	// In TempDir: check must not create the output dir, and the config dir is versioned.
	sum := sha256.Sum256([]byte(filepath.Clean(abs)))
	return filepath.Join(os.TempDir(), "loc-"+hex.EncodeToString(sum[:])[:16]+".lock"), nil
}
