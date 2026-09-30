//go:build windows

package main

import (
	"errors"
	"syscall"
)

// isBusyError: the Windows errors a scanner or indexer causes while it holds a file open.
func isBusyError(err error) bool {
	var e syscall.Errno
	if !errors.As(err, &e) {
		return false
	}
	const (
		errAccessDenied     = syscall.Errno(5)
		errSharingViolation = syscall.Errno(32)
		errLockViolation    = syscall.Errno(33)
	)
	return e == errAccessDenied || e == errSharingViolation || e == errLockViolation
}
