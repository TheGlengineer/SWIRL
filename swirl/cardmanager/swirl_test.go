package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func quadImage(s int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, s, s))
	cols := []color.NRGBA{{248, 0, 0, 255}, {0, 252, 0, 255}, {0, 0, 248, 255}, {248, 252, 248, 255}}
	for y := 0; y < s; y++ {
		for x := 0; x < s; x++ {
			q := 0
			if x >= s/2 {
				q++
			}
			if y >= s/2 {
				q += 2
			}
			img.SetNRGBA(x, y, cols[q])
		}
	}
	return img
}

// VQ encoder good enough for images made of flat 2x2 blocks
func encodeVQ(img *image.NRGBA) []byte {
	w := img.Bounds().Dx()
	cb := make([]byte, 2048)
	idx := make([]byte, (w/2)*(w/2))
	seen := map[[4]uint16]int{}
	for by := 0; by < w/2; by++ {
		for bx := 0; bx < w/2; bx++ {
			var k [4]uint16
			for t := 0; t < 4; t++ {
				c := img.NRGBAAt(bx*2+(t>>1), by*2+(t&1))
				k[t] = uint16(c.R>>3)<<11 | uint16(c.G>>2)<<5 | uint16(c.B>>3)
			}
			e, ok := seen[k]
			if !ok {
				e = len(seen)
				seen[k] = e
				for t := 0; t < 4; t++ {
					binary.LittleEndian.PutUint16(cb[e*8+t*2:], k[t])
				}
			}
			idx[twiddleIdx(bx, by)] = byte(e)
		}
	}
	hdr := make([]byte, 32)
	copy(hdr, "GBIX")
	copy(hdr[16:], "PVRT")
	hdr[24], hdr[25] = 1, 3
	binary.LittleEndian.PutUint16(hdr[28:], uint16(w))
	binary.LittleEndian.PutUint16(hdr[30:], uint16(w))
	return append(append(hdr, cb...), idx...)
}

func TestPVRRoundTrip(t *testing.T) {
	src := quadImage(256)
	for name, b := range map[string][]byte{"565": encodePVR565(src, 256), "vq": encodeVQ(src)} {
		img, err := decodePVR(b)
		if err != nil {
			t.Fatal(name, err)
		}
		for _, p := range [][2]int{{10, 10}, {200, 10}, {10, 200}, {200, 200}} {
			a, c := img.NRGBAAt(p[0], p[1]), src.NRGBAAt(p[0], p[1])
			if a.R>>3 != c.R>>3 || a.G>>2 != c.G>>2 || a.B>>3 != c.B>>3 {
				t.Fatalf("%s pixel %v: got %v want %v", name, p, a, c)
			}
		}
	}
}

func ipSector(name, product string) []byte {
	b := make([]byte, 32768)
	for i := 0; i < 0x100; i++ {
		b[i] = ' '
	}
	copy(b, "SEGA SEGAKATANA ")
	copy(b[0x25:], "CD-ROM1/1")
	copy(b[0x30:], "JUE     ")
	copy(b[0x38:], "E000F10")
	copy(b[0x40:], product)
	copy(b[0x4A:], "V1.000")
	copy(b[0x50:], "20010101")
	copy(b[0x80:], name)
	return b
}

func TestCDIDiscArt(t *testing.T) {
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	os.MkdirAll(data, 0o755)
	os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), make([]byte, 5000), 0o644)
	os.WriteFile(filepath.Join(data, "0GDTEX.PVR"), encodeVQ(quadImage(256)), 0o644)
	iso := filepath.Join(dir, "s.iso")
	const base = 11702
	if err := buildISO(data, iso, base, "TEST", ipSector("CDI TEST GAME", "T-99901N")); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(iso)
	// wrap as a CDI style image: junk audio first, then 2336 byte Mode 2 sectors (8 byte subheader)
	game := filepath.Join(dir, "03")
	os.MkdirAll(game, 0o755)
	f, _ := os.Create(filepath.Join(game, "game.cdi"))
	f.Write(make([]byte, 2352*300))
	for i := 0; i < len(raw)/2048; i++ {
		f.Write(make([]byte, 8))
		f.Write(raw[i*2048 : (i+1)*2048])
		f.Write(make([]byte, 280))
	}
	f.Close()
	ip, format, err := readImageIP(game)
	if err != nil || ip.Name != "CDI TEST GAME" {
		t.Fatal("ip", ip, format, err)
	}
	b, err := readDiscFile(game, "0GDTEX.PVR", 4<<20)
	if err != nil {
		t.Fatal(err)
	}
	img, err := decodePVR(b)
	if err != nil || img.NRGBAAt(200, 200).R < 240 {
		t.Fatal("art", err)
	}
	v := makeVMU(img, false)
	if len(v) != vmuBytes {
		t.Fatal("vmu size")
	}
}

func TestDatRoundTrip(t *testing.T) {
	d := newDat(metaSize)
	d.Set("T1234N", encodeMeta(Meta{Players: 4, VMUBlocks: 9, Network: 2, Genre: 5, Accessories: 3, Description: "Hello ’world’"}))
	d.Set("MK51000", encodeMeta(Meta{Players: 1}))
	p := filepath.Join(t.TempDir(), "META.DAT")
	if err := d.Write(p); err != nil {
		t.Fatal(err)
	}
	r, err := readDat(p)
	if err != nil {
		t.Fatal(err)
	}
	m := decodeMeta(r.Chunks["T1234N"])
	if m.Players != 4 || m.VMUBlocks != 9 || m.Network != 2 || m.Genre != 5 || m.Accessories != 3 || m.Description != "Hello 'world'" {
		t.Fatalf("%+v", m)
	}
}
