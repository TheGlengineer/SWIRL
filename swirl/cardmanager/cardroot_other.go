//go:build !linux

package main

// cardRootErr reports why a path cannot be used as a card root. On Windows and macOS every
// path is allowed through here; the OS-specific guards (system disk, internal drive) live in
// the format/diskInfo paths instead.
func cardRootErr(root string) string { return "" }
