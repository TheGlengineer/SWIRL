package main

// macOS keeps extra file information (such as where a file came from) in hidden "._name" files when it
// writes to a FAT32 card. SWIRL and GDEMU have no use for them, and a "._disc.gdi" next to "disc.gdi" can
// be mistaken for the disc, so on a Mac they are removed from the card whenever it is read.

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var appleDoubleMagic = []byte{0x00, 0x05, 0x16, 0x07}

// isAppleDouble reports whether p is one of those "._" files (checked by its header, so a real file
// that happens to start with "._" is left alone).
func isAppleDouble(p string, size int64) bool {
	if !strings.HasPrefix(filepath.Base(p), "._") || size > 4<<20 {
		return false
	}
	f, err := os.Open(p)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4)
	if n, _ := f.Read(head); n != 4 {
		return size == 0
	}
	return bytes.Equal(head, appleDoubleMagic)
}

// cleanAppleDouble removes macOS "._" files from the card and returns how many it removed.
func cleanAppleDouble(root string) int {
	n := 0
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (name == ".Spotlight-V100" || name == ".Trashes" || name == ".fseventsd" || name == ".TemporaryItems" || strings.HasPrefix(name, "$")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasPrefix(name, "._") {
			return nil
		}
		if info, err := d.Info(); err == nil && isAppleDouble(p, info.Size()) && os.Remove(p) == nil {
			n++
		}
		return nil
	})
	return n
}

// tidyMacFiles cleans the card on a Mac; elsewhere it does nothing.
func tidyMacFiles(root string) {
	if runtime.GOOS == "darwin" {
		cleanAppleDouble(root)
	}
}
