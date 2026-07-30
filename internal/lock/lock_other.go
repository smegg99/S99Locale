// internal/lock/lock_other.go

//go:build !unix

package lock

import (
	"errors"
	"os"
)

// Nothing here locks: loc targets Linux.
var errWouldBlock = errors.New("lock: would block")

// acquire always succeeds; the lock file is still created, so the rest of the path is identical.
func acquire(*os.File, bool) error { return nil }

func release(*os.File) error { return nil }
