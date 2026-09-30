//go:build !windows

package main

func isBusyError(err error) bool { return false }
