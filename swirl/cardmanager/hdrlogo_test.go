package main

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

// The sheet: the orange logo in the top band as it is in the theme picture, the blue one below with the
// swirl blue and the text untouched, 32 KB, square twiddled RGB565 the menu's loader reads.
func TestHeaderLogoSheet(t *testing.T) {
	bg := os.Getenv("SWIRL_BG_U_L")
	if bg == "" {
		bg = filepath.Join(appDataDir(), "openmenu", "BG_U_L.PVR")
	}
	src, err := os.ReadFile(bg)
	if err != nil {
		t.Skip("no cached BG_U_L.PVR:", err)
	}
	sheet, err := buildHeaderLogoSheet(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheet) != 32+hdrLogoSize*hdrLogoSize*2 {
		t.Fatalf("sheet is %d bytes", len(sheet))
	}
	img, err := decodePVR(sheet)
	if err != nil {
		t.Fatal(err)
	}
	orig, _ := decodePVR(src)
	orange, blue, text := 0, 0, 0
	for y := 0; y < hdrLogoH; y++ {
		for x := 0; x < hdrLogoW; x++ {
			o := orig.NRGBAAt(hdrLogoSrcX+x, hdrLogoSrcY+y)
			top := img.NRGBAAt(x, y)
			bot := img.NRGBAAt(x, hdrLogoBand+y)
			if near(top, o) == false {
				t.Fatalf("top band differs from the theme picture at %d,%d: %v vs %v", x, y, top, o)
			}
			_, s, v := rgbToHSV(o)
			if s > 0.18 && v > 0.15 {
				orange++
				if !(bot.B > bot.R+40 && bot.B > bot.G) {
					t.Fatalf("swirl pixel not blue at %d,%d: %v", x, y, bot)
				}
				blue++
			} else if !near(bot, o) {
				t.Fatalf("text or plate changed in the blue band at %d,%d: %v vs %v", x, y, bot, o)
			} else {
				text++
			}
		}
	}
	if orange < 80 || blue != orange || text < 1000 {
		t.Fatalf("orange %d blue %d text %d", orange, blue, text)
	}
	// outside the two logos the sheet is the plate colour
	if c := img.NRGBAAt(120, 100); c.R < 240 || c.G < 240 || c.B < 240 {
		t.Fatalf("stray colour outside the logos: %v", c)
	}
}

// RGB565 loses the low bits; compare within that
func near(a, b color.NRGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) <= 8 && d(a.G, b.G) <= 4 && d(a.B, b.B) <= 8
}
