package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppleDoubleCleanup(t *testing.T) {
	card := t.TempDir()
	os.MkdirAll(filepath.Join(card, "01"), 0o755)
	os.MkdirAll(filepath.Join(card, "02"), 0o755)
	ad := append([]byte{0, 5, 0x16, 7}, make([]byte, 60)...)
	os.WriteFile(filepath.Join(card, "01", "disc.gdi"), []byte("3\n"), 0o644)
	os.WriteFile(filepath.Join(card, "01", "._disc.gdi"), ad, 0o644)
	os.WriteFile(filepath.Join(card, "02", "._track01.bin"), ad, 0o644)
	os.WriteFile(filepath.Join(card, "._GDEMU.INI"), ad, 0o644)
	os.WriteFile(filepath.Join(card, "02", "._notes.txt"), []byte("a real file"), 0o644)

	if g := findGDI(filepath.Join(card, "01")); filepath.Base(g) != "disc.gdi" {
		t.Fatal("findGDI picked", g)
	}
	if n := cleanAppleDouble(card); n != 3 {
		t.Fatal("removed", n)
	}
	if fileExists(filepath.Join(card, "01", "._disc.gdi")) || !fileExists(filepath.Join(card, "01", "disc.gdi")) {
		t.Fatal("wrong files removed")
	}
	if !fileExists(filepath.Join(card, "02", "._notes.txt")) {
		t.Fatal("a file that is not AppleDouble was removed")
	}
}
