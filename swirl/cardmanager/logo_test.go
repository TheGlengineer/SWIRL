package main

import (
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"testing"
)

// LOGO_PIC=picture.png [LOGO_CARD=card root] go test -run TestLogoFromPicture
func TestLogoFromPicture(t *testing.T) {
	f, err := os.Open(os.Getenv("LOGO_PIC"))
	if err != nil {
		t.Skip()
	}
	img, _, _ := image.Decode(f)
	f.Close()
	for i, o := range []LogoOptions{{Fit: "crop", Threshold: -1}, {Fit: "crop", Threshold: -1, Dither: true}, {Fit: "fit", Threshold: -1}} {
		os.WriteFile(filepath.Join(os.TempDir(), "logo_opt"+string(rune('0'+i))+".png"), vmuPNG(makeLogo(img, o)), 0o644)
	}
	if root := os.Getenv("LOGO_CARD"); root != "" {
		if err := SaveLogo(root, makeLogo(img, LogoOptions{Fit: "crop", Threshold: -1})); err != nil {
			t.Fatal(err)
		}
	}
}
