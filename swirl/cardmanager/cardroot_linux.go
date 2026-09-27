//go:build linux

package main

// Guarding against the classic Linux mistake: treating the filesystem root as the SD card.
// On Linux the drive list starts with "Computer" (path "/"), and if that is scanned as a card,
// Add games / Install SWIRL try to write 02/, 03/ ... folders onto the system disk.

import (
	"fmt"
	"path/filepath"
)

// cardRootErr reports why a path cannot be used as a card root, or "" when it can.
func cardRootErr(root string) string {
	if filepath.Clean(root) == string(filepath.Separator) {
		return fmt.Sprintf("%s is the whole computer, not an SD card. Pick the card from the list; it appears under /run/media or /media once it is in the reader.", root)
	}
	return ""
}
