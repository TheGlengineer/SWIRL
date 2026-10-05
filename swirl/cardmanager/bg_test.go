package main

import (
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"testing"

	"hash/crc32"
)

// a picture with gradients, an edge and flat areas: what a backdrop photo looks like to a VQ encoder
func testPicture(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{uint8(x * 255 / w), uint8(y * 255 / h), uint8(128 + 100*math.Sin(float64(x+y)/40)), 255}
			if (x/64+y/64)%7 == 0 {
				c = color.NRGBA{20, 24, 40, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func TestVQRoundTrip(t *testing.T) {
	src := testPicture(512, 512)
	pvr := encodePVRVQ(src)
	if len(pvr) != 32+2048+512*512/4 {
		t.Fatalf("VQ texture is %d bytes", len(pvr))
	}
	dec, err := decodePVR(pvr)
	if err != nil {
		t.Fatal(err)
	}
	if dec.Bounds().Dx() != 512 || dec.Bounds().Dy() != 512 {
		t.Fatal("decoded size", dec.Bounds())
	}
	// PSNR against the source: VQ with 256 entries on a smooth picture should be well above 28 dB
	var se float64
	for y := 0; y < 512; y++ {
		for x := 0; x < 512; x++ {
			a, b := src.NRGBAAt(x, y), dec.NRGBAAt(x, y)
			for _, d := range []float64{float64(a.R) - float64(b.R), float64(a.G) - float64(b.G), float64(a.B) - float64(b.B)} {
				se += d * d
			}
		}
	}
	mse := se / (512 * 512 * 3)
	psnr := 10 * math.Log10(255*255/mse)
	t.Logf("VQ PSNR %.1f dB", psnr)
	if psnr < 28 {
		t.Fatalf("VQ quality too low: %.1f dB", psnr)
	}
	// the menu's loader reads the same header fields: VQ twiddled RGB565, 512 x 512
	if pvr[24] != 1 || pvr[25] != 3 || pvr[28] != 0 || pvr[29] != 2 {
		t.Fatalf("header %x", pvr[24:32])
	}
}

func TestBgPictures(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, editsDir), 0o755)
	if l := ListBg(root); len(l) != 0 {
		t.Fatal("pictures on an empty card")
	}
	n, err := AddBg(root, testPicture(640, 480), "  Sunset over the bay, a long name  ", 4, 0)
	if err != nil || n != 1 {
		t.Fatal(n, err)
	}
	n2, err := AddBg(root, testPicture(320, 240), "Beach", 7, 0)
	if err != nil || n2 != 2 {
		t.Fatal(n2, err)
	}
	l := ListBg(root)
	if len(l) != 2 || l[0].Name != "Sunset over the bay" || l[0].Dim != 4 || l[1].Name != "Beach" || l[1].Dim != 7 {
		t.Fatalf("list %+v", l)
	}
	// the index matches the files byte for byte (size and checksum), as the console checks
	ents := readBgDat(bgDat(root))
	b, _ := os.ReadFile(bgPVR(root, 1))
	if int(ents[0].Size) != len(b) || ents[0].CRC != crc32.ChecksumIEEE(b) {
		t.Fatal("index does not match the file")
	}
	if dec, err := decodePVR(b); err != nil || dec.Bounds().Dx() != 512 {
		t.Fatal("stored texture", err)
	}
	// rename and darken
	if err := RenameBg(root, 2, "Beach at night", 9); err != nil {
		t.Fatal(err)
	}
	if l = ListBg(root); l[1].Name != "Beach at night" || l[1].Dim != 9 || l[0].Dim != 4 {
		t.Fatalf("after rename %+v", l)
	}
	// a replaced slot keeps its number
	if n, err = AddBg(root, testPicture(512, 512), "Sunset two", 2, 1); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if l = ListBg(root); len(l) != 2 || l[0].Name != "Sunset two" || l[0].Dim != 2 {
		t.Fatalf("after replace %+v", l)
	}
	// removing the first closes the gap: Beach becomes picture 1
	if err := RemoveBg(root, 1); err != nil {
		t.Fatal(err)
	}
	if l = ListBg(root); len(l) != 1 || l[0].N != 1 || l[0].Name != "Beach at night" || !fileExists(bgPVR(root, 1)) || fileExists(bgPVR(root, 2)) {
		t.Fatalf("after remove %+v", l)
	}
	// the cap
	for i := 0; i < bgMax-1; i++ {
		if _, err := AddBg(root, testPicture(64, 48), "", 4, 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AddBg(root, testPicture(64, 48), "one too many", 4, 0); err == nil {
		t.Fatal("ninth picture accepted")
	}
	if l = ListBg(root); l[7].Name != "Picture 8" {
		t.Fatalf("default name %+v", l[7])
	}
	// onto the disc
	data := t.TempDir()
	var lines []string
	if err := addBgToDisc(root, data, func(f string, a ...any) { lines = append(lines, f) }); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(data, "BG", "BG.DAT")) || !fileExists(filepath.Join(data, "BG", "BG08.PVR")) || len(lines) != 1 {
		t.Fatal("disc files", lines)
	}
	if got := readBgDat(filepath.Join(data, "BG", "BG.DAT")); len(got) != 8 {
		t.Fatal("disc index")
	}
	// a card with no pictures leaves nothing on the disc
	if err := addBgToDisc(t.TempDir(), data, func(string, ...any) {}); err != nil || fileExists(filepath.Join(data, "BG")) {
		t.Fatal("stale BG folder on the disc")
	}
}
