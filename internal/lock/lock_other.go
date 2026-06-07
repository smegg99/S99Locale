// internal/lock/lock_other.go

//go:build !unix

package lock

import (
	"errors"
	"os"
)

// Nothing here locks. loc targets Linux laptops and Linux CI runners - `just
// install` shells out to `sudo install` - and a half-working lock on a platform
// nobody runs is worse than an honest absence: it would read as a guarantee.
//
// The file is still created, so the rest of the code path is identical.
var errWouldBlock = errors.New("lock: would block")

func acquire(*os.File, bool, bool) error { return nil }

func release(*os.File) error { return nil }
