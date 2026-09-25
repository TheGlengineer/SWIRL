package main

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/png"
	"path/filepath"
	"testing"
)

func TestGameThumbs(t *testing.T) {
	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "GAME A", "T-00001N")
	if err := installSwirl(card, "", false, t.Logf); err != nil {
		t.Fatal(err)
	}
	// no art yet
	if _, err := GameThumb(card, "02", "T00001N", false); err == nil {
		t.Fatal("expected no art")
	}
	// the owner's own box art
	img := image.NewNRGBA(image.Rect(0, 0, 300, 300))
	if err := SaveGame(SaveGameRequest{Root: card, Folder: "02", Name: "Game A", Region: "JUE", VGA: true, Box: "data:image/png;base64," + b64(pngBytes(img))}); err != nil {
		t.Fatal(err)
	}
	for _, big := range []bool{false, true} {
		b, err := GameThumb(card, "02", "T00001N", big)
		if err != nil {
			t.Fatal(err)
		}
		m, _, err := image.Decode(bytes.NewReader(b))
		want := 128
		if big {
			want = 256
		}
		if err != nil || m.Bounds().Dx() != want {
			t.Fatalf("thumb size %v %v", m.Bounds(), err)
		}
	}
	// after Update SWIRL the art comes from the menu disc's ICON.DAT / BOX.DAT
	if err := installSwirl(card, "", false, t.Logf); err != nil {
		t.Fatal(err)
	}
	if _, err := GameThumb(card, "../x", "", false); err == nil {
		t.Fatal("bad folder accepted")
	}
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
