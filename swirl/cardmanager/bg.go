package main

// Backdrop pictures (2.17): the owner's own pictures behind the SWIRL menu. The window crops a picture to
// 4:3, darkens it and bakes the readability scrim; this file turns the result into what the console reads:
// a 512 x 512 VQ compressed PVR texture (about 66 KB of video memory, the Dreamcast stretches it to the
// screen) and one entry in BG.DAT, a small binary index the menu reads with fixed sizes and a checksum per
// picture (no text parsing on the console). The files live in the card's SWIRL/BG folder and are copied onto
// the menu disc at Update SWIRL as BG/BG.DAT and BG/BGnn.PVR.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	bgDir      = "BG"
	bgMax      = 8   // SW_BG_MAX in the menu
	bgSize     = 512 // texture side
	bgNameMax  = 19  // bytes of UTF-8 in the 20 byte field
	bgDatMagic = "SWBG"
)

// BgEntry is one picture as the window lists it.
type BgEntry struct {
	N    int    `json:"n"`
	Name string `json:"name"`
	Dim  int    `json:"dim"`
	Size int    `json:"size"`
}

func bgRoot(root string) string { return filepath.Join(root, editsDir, bgDir) }
func bgPVR(root string, n int) string {
	return filepath.Join(bgRoot(root), fmt.Sprintf("BG%02d.PVR", n))
}
func bgPNG(root string, n int) string {
	return filepath.Join(bgRoot(root), fmt.Sprintf("BG%02d.PNG", n))
}
func bgDat(root string) string { return filepath.Join(bgRoot(root), "BG.DAT") }

// writeFileSync writes a file the way the card's other small files are written: whole, then synced.
func writeFileSync(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// bgIndexEntry mirrors bg_entry in ui/swirl/sw_backdrop.c: 32 bytes each.
type bgIndexEntry struct {
	Name [20]byte
	Dim  uint8
	Pad  [3]byte
	Size uint32
	CRC  uint32
}

// readBgDat returns the entries of a BG.DAT, or nil for a missing or unusable file.
func readBgDat(path string) []bgIndexEntry {
	b, err := os.ReadFile(path)
	if err != nil || len(b) < 12 || string(b[:4]) != bgDatMagic || binary.LittleEndian.Uint32(b[4:]) != 1 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b[8:]))
	if n < 1 || n > bgMax || len(b) < 12+n*32 {
		return nil
	}
	out := make([]bgIndexEntry, n)
	for i := range out {
		e := b[12+i*32:]
		copy(out[i].Name[:], e[:20])
		out[i].Dim = e[20]
		out[i].Size = binary.LittleEndian.Uint32(e[24:])
		out[i].CRC = binary.LittleEndian.Uint32(e[28:])
	}
	return out
}

func writeBgDat(path string, entries []bgIndexEntry) error {
	b := make([]byte, 12+len(entries)*32)
	copy(b, bgDatMagic)
	binary.LittleEndian.PutUint32(b[4:], 1)
	binary.LittleEndian.PutUint32(b[8:], uint32(len(entries)))
	for i, e := range entries {
		o := b[12+i*32:]
		copy(o[:20], e.Name[:])
		o[20] = e.Dim
		binary.LittleEndian.PutUint32(o[24:], e.Size)
		binary.LittleEndian.PutUint32(o[28:], e.CRC)
	}
	return writeFileSync(path, b)
}

func bgEntryName(e bgIndexEntry) string {
	s := string(e.Name[:])
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	return s
}

// ListBg lists the pictures on a card, in the order the console shows them.
func ListBg(root string) []BgEntry {
	var out []BgEntry
	for i, e := range readBgDat(bgDat(root)) {
		out = append(out, BgEntry{N: i + 1, Name: bgEntryName(e), Dim: int(e.Dim), Size: int(e.Size)})
	}
	return out
}

// bgCleanName keeps a name the console can show: at most 19 bytes of UTF-8, no control characters.
func bgCleanName(name string, n int) string {
	name = strings.TrimSpace(name)
	var sb strings.Builder
	for _, r := range name {
		if r < 32 || r == 127 {
			continue
		}
		if sb.Len()+utf8.RuneLen(r) > bgNameMax {
			break
		}
		sb.WriteRune(r)
	}
	if sb.Len() == 0 {
		return fmt.Sprintf("Picture %d", n)
	}
	return sb.String()
}

// rebuildBgDat writes BG.DAT from the PVR files present (1..n with no gaps), keeping names and dim values.
func rebuildBgDat(root string, names map[int]string, dims map[int]int) error {
	old := readBgDat(bgDat(root))
	var entries []bgIndexEntry
	for n := 1; n <= bgMax; n++ {
		b, err := os.ReadFile(bgPVR(root, n))
		if err != nil {
			break
		}
		var e bgIndexEntry
		name, dim := "", 4
		if n-1 < len(old) {
			name, dim = bgEntryName(old[n-1]), int(old[n-1].Dim)
		}
		if v, ok := names[n]; ok {
			name = v
		}
		if v, ok := dims[n]; ok {
			dim = v
		}
		copy(e.Name[:], bgCleanName(name, n))
		e.Dim = uint8(dim)
		e.Size = uint32(len(b))
		e.CRC = crc32.ChecksumIEEE(b)
		entries = append(entries, e)
	}
	if len(entries) == 0 {
		os.Remove(bgDat(root))
		return nil
	}
	return writeBgDat(bgDat(root), entries)
}

// AddBg stores a processed picture (already 4:3, darkened and scrimmed by the window) as the next slot, or
// replaces slot n when n is 1..count. Returns the slot used.
func AddBg(root string, img image.Image, name string, dim int, n int) (int, error) {
	if dim < 0 || dim > 10 {
		return 0, errors.New("the darkening must be 0 to 10")
	}
	count := len(ListBg(root))
	if n < 1 || n > count {
		if count >= bgMax {
			return 0, fmt.Errorf("a card holds up to %d backdrop pictures; remove one first", bgMax)
		}
		n = count + 1
	}
	if err := os.MkdirAll(bgRoot(root), 0o755); err != nil {
		return 0, err
	}
	sq := resample(img, img.Bounds(), bgSize, bgSize)
	pvr := encodePVRVQ(sq)
	if err := writeFileSync(bgPVR(root, n), pvr); err != nil {
		return 0, err
	}
	if err := writeFileSync(bgPNG(root, n), pngBytes(sq)); err != nil {
		return 0, err
	}
	return n, rebuildBgDat(root, map[int]string{n: name}, map[int]int{n: dim})
}

// RenameBg changes a picture's name or darkening.
func RenameBg(root string, n int, name string, dim int) error {
	if n < 1 || n > len(ListBg(root)) {
		return errors.New("no such picture")
	}
	if dim < 0 || dim > 10 {
		return errors.New("the darkening must be 0 to 10")
	}
	return rebuildBgDat(root, map[int]string{n: name}, map[int]int{n: dim})
}

// RemoveBg drops a picture and closes the gap, so the console's numbering stays 1..count.
func RemoveBg(root string, n int) error {
	list := ListBg(root)
	if n < 1 || n > len(list) {
		return errors.New("no such picture")
	}
	names, dims := map[int]string{}, map[int]int{}
	for _, e := range list {
		if e.N > n {
			names[e.N-1], dims[e.N-1] = e.Name, e.Dim
		} else if e.N < n {
			names[e.N], dims[e.N] = e.Name, e.Dim
		}
	}
	os.Remove(bgPVR(root, n))
	os.Remove(bgPNG(root, n))
	for i := n + 1; i <= len(list); i++ {
		os.Rename(bgPVR(root, i), bgPVR(root, i-1))
		os.Rename(bgPNG(root, i), bgPNG(root, i-1))
	}
	return rebuildBgDat(root, names, dims)
}

// addBgToDisc copies the pictures and their index into the menu disc's data folder (Update SWIRL).
func addBgToDisc(root, data string, log func(string, ...any)) error {
	dst := filepath.Join(data, bgDir)
	os.RemoveAll(dst) // what an older disc carried
	list := ListBg(root)
	if len(list) == 0 {
		return nil
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	if err := copyFile(bgDat(root), filepath.Join(dst, "BG.DAT")); err != nil {
		return err
	}
	for _, e := range list {
		if err := copyFile(bgPVR(root, e.N), filepath.Join(dst, fmt.Sprintf("BG%02d.PVR", e.N))); err != nil {
			return err
		}
	}
	if len(list) == 1 {
		log("Added your backdrop picture (%s)", list[0].Name)
	} else {
		log("Added your %d backdrop pictures", len(list))
	}
	return nil
}

// ---------- VQ compression ----------

// encodePVRVQ makes a square VQ compressed RGB565 texture (PVR type 0x03, twiddled indices) from a square
// picture whose side is a power of two: a 256 entry codebook of 2 x 2 texel blocks found by k-means over the
// picture's blocks, then one index byte per block. A 512 x 512 picture comes out at 65,568 bytes of texture
// against 524,288 uncompressed, which is what makes a full screen backdrop affordable in video memory.
func encodePVRVQ(img *image.NRGBA) []byte {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()
	nb := (w / 2) * (h / 2)
	// every block as 12 floats (4 texels of r, g, b in 0..255)
	blocks := make([][12]float32, nb)
	for by := 0; by < h/2; by++ {
		for bx := 0; bx < w/2; bx++ {
			var v [12]float32
			for k := 0; k < 4; k++ {
				c := img.NRGBAAt(bx*2+(k>>1), by*2+(k&1))
				v[k*3], v[k*3+1], v[k*3+2] = float32(c.R), float32(c.G), float32(c.B)
			}
			blocks[by*(w/2)+bx] = v
		}
	}
	book := vqCodebook(blocks, 256)
	// the texture: codebook as RGB565 little endian, then the indices in twiddled block order
	out := make([]byte, 2048+nb)
	for i, b := range book {
		for k := 0; k < 4; k++ {
			v := rgb565(uint8(b[k*3]+0.5), uint8(b[k*3+1]+0.5), uint8(b[k*3+2]+0.5))
			binary.LittleEndian.PutUint16(out[i*8+k*2:], v)
		}
	}
	for by := 0; by < h/2; by++ {
		for bx := 0; bx < w/2; bx++ {
			out[2048+twiddleIdx(bx, by)] = uint8(vqNearest(book, blocks[by*(w/2)+bx]))
		}
	}
	hdr := make([]byte, 32)
	copy(hdr, "GBIX")
	binary.LittleEndian.PutUint32(hdr[4:], 8)
	copy(hdr[16:], "PVRT")
	binary.LittleEndian.PutUint32(hdr[20:], uint32(len(out)+8))
	hdr[24] = 0x01 // RGB565
	hdr[25] = 0x03 // VQ, twiddled
	binary.LittleEndian.PutUint16(hdr[28:], uint16(w))
	binary.LittleEndian.PutUint16(hdr[30:], uint16(h))
	return append(hdr, out...)
}

func rgb565(r, g, b uint8) uint16 {
	return uint16(r>>3)<<11 | uint16(g>>2)<<5 | uint16(b>>3)
}

func vqDist(a, b *[12]float32) float32 {
	var d float32
	for i := 0; i < 12; i++ {
		x := a[i] - b[i]
		d += x * x
	}
	return d
}

func vqNearest(book [][12]float32, v [12]float32) int {
	best, bd := 0, float32(1e30)
	for i := range book {
		if d := vqDist(&book[i], &v); d < bd {
			best, bd = i, d
		}
	}
	return best
}

// vqCodebook: k-means with a deterministic start (every (n / k)th block of the picture sorted by brightness,
// so a gradient gets evenly spread entries), twelve rounds, empty entries re-seeded from the worst fitting
// blocks so no codebook entry is wasted.
func vqCodebook(blocks [][12]float32, k int) [][12]float32 {
	n := len(blocks)
	if n == 0 {
		return make([][12]float32, k)
	}
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	bright := func(b *[12]float32) float32 {
		return b[0] + b[1] + b[2] + b[3] + b[4] + b[5] + b[6] + b[7] + b[8] + b[9] + b[10] + b[11]
	}
	sort.Slice(order, func(i, j int) bool { return bright(&blocks[order[i]]) < bright(&blocks[order[j]]) })
	book := make([][12]float32, k)
	for i := 0; i < k; i++ {
		book[i] = blocks[order[i*n/k]]
	}
	rnd := rand.New(rand.NewSource(1))
	assign := make([]int, n)
	for round := 0; round < 12; round++ {
		var sums [256][12]float32
		var counts [256]int
		var worst [256]float32
		var worstIdx [256]int
		for i := range blocks {
			j := vqNearest(book, blocks[i])
			assign[i] = j
			counts[j]++
			for c := 0; c < 12; c++ {
				sums[j][c] += blocks[i][c]
			}
			if d := vqDist(&book[j], &blocks[i]); d > worst[j] {
				worst[j], worstIdx[j] = d, i
			}
		}
		for j := 0; j < k; j++ {
			if counts[j] == 0 {
				// re-seed from the worst fitting block of the fullest entry
				big := 0
				for m := 1; m < k; m++ {
					if counts[m] > counts[big] {
						big = m
					}
				}
				if counts[big] > 1 {
					book[j] = blocks[worstIdx[big]]
				} else {
					book[j] = blocks[rnd.Intn(n)]
				}
				continue
			}
			for c := 0; c < 12; c++ {
				book[j][c] = sums[j][c] / float32(counts[j])
			}
		}
	}
	return book
}

// bgPreviewColor is used by tests: the average colour of a decoded texture.
func bgPreviewColor(img *image.NRGBA) color.NRGBA {
	var r, g, b, n uint64
	for y := 0; y < img.Bounds().Dy(); y += 8 {
		for x := 0; x < img.Bounds().Dx(); x += 8 {
			c := img.NRGBAAt(x, y)
			r += uint64(c.R)
			g += uint64(c.G)
			b += uint64(c.B)
			n++
		}
	}
	return color.NRGBA{uint8(r / n), uint8(g / n), uint8(b / n), 255}
}
