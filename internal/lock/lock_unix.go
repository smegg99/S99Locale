// internal/lock/lock_unix.go

//go:build unix

package lock

import (
	"os"
	"syscall"
)

// flock ties the lock to the open file description rather than the process, so
// the kernel drops it when loc exits however it exits. There is no stale lock
// to detect and no PID file to reap.
var errWouldBlock = syscall.EWOULDBLOCK

func acquire(f *os.File, exclusive, block bool) error {
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	if !block {
		how |= syscall.LOCK_NB
	}
	return syscall.Flock(int(f.Fd()), how)
}

func release(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
