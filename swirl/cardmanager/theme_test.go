package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func menuMusic(t *testing.T, card string) []byte {
	b, err := readDiscFile(filepath.Join(card, "01"), "BGM.ADP", 32<<20)
	if err != nil {
		t.Fatal("menu disc has no BGM.ADP:", err)
	}
	return b
}

func TestThemeIsDefaultMusic(t *testing.T) {
	start := time.Now()
	theme, err := defaultMusic()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("theme converted in %v, %.1f MB", time.Since(start), float64(len(theme))/(1<<20))
	if string(theme[:4]) != "OMBG" || len(theme) < 8<<20 {
		t.Fatalf("theme looks wrong: %d bytes", len(theme))
	}
	card := t.TempDir()
	logf := func(f string, a ...any) { t.Logf(f, a...) }
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "GAME", "T-00001N")

	// no music of their own: the theme goes on the disc
	if err := installSwirl(card, "", false, logf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(menuMusic(t, card), theme) {
		t.Fatal("the theme is not on the menu disc")
	}
	if GetMusicInfo(card).Present {
		t.Fatal("the theme must not count as the owner's music")
	}

	// their own music replaces it
	own := []byte("OMBG\x01\x00\x00\x00\x44\xac\x00\x00\x02\x00" + string(make([]byte, 2000)))
	os.MkdirAll(filepath.Join(card, editsDir), 0o755)
	os.WriteFile(musicPath(card), own, 0o644)
	if err := installSwirl(card, "", true, logf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(menuMusic(t, card), own) {
		t.Fatal("own music not used")
	}

	// removing it brings the theme back, even though their music is still on the old menu disc;
	// the old "no music" choice is ignored
	RemoveMusic(card)
	os.WriteFile(filepath.Join(card, editsDir, "BGM.NONE"), []byte("x"), 0o644)
	if err := installSwirl(card, "", true, logf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(menuMusic(t, card), theme) {
		t.Fatal("theme did not come back")
	}
	if fileExists(filepath.Join(card, editsDir, "BGM.NONE")) {
		t.Fatal("BGM.NONE left behind")
	}
}
