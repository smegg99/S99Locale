// internal/lock/lock_unix.go

//go:build unix

package lock

import (
	"os"
	"syscall"
)

// flock ties the lock to the open file description, so the kernel drops it however loc exits.
var errWouldBlock = syscall.EWOULDBLOCK

// acquire takes the lock or returns errWouldBlock at once; blocking flock cannot be interrupted.
func acquire(f *os.File, exclusive bool) error {
	how := syscall.LOCK_SH
	if exclusive {
		how = syscall.LOCK_EX
	}
	return syscall.Flock(int(f.Fd()), how|syscall.LOCK_NB)
}

func release(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
