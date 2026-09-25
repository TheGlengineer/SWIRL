package main

// PVR texture decode/encode and the image helpers used for box art, icons and VMU screens.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/png"
)

func twiddleIdx(x, y int) int {
	r := 0
	for i := 0; i < 11; i++ {
		r |= ((y >> i) & 1) << (2 * i)
		r |= ((x >> i) & 1) << (2*i + 1)
	}
	return r
}

func texel(v uint16, pf byte) color.NRGBA {
	switch pf {
	case 1: // RGB565
		return color.NRGBA{uint8((v >> 11 & 31) * 255 / 31), uint8((v >> 5 & 63) * 255 / 63), uint8((v & 31) * 255 / 31), 255}
	case 2: // ARGB4444
		return color.NRGBA{uint8((v >> 8 & 15) * 17), uint8((v >> 4 & 15) * 17), uint8((v & 15) * 17), uint8((v >> 12 & 15) * 17)}
	default: // ARGB1555
		a := uint8(0)
		if v&0x8000 != 0 {
			a = 255
		}
		return color.NRGBA{uint8((v >> 10 & 31) * 255 / 31), uint8((v >> 5 & 31) * 255 / 31), uint8((v & 31) * 255 / 31), a}
	}
}

// decodePVR handles RGB565 / ARGB1555 / ARGB4444 in twiddled, rectangle and VQ layouts (with or without mipmaps).
func decodePVR(b []byte) (*image.NRGBA, error) {
	p := bytes.Index(b, []byte("PVRT"))
	if p < 0 || len(b) < p+16 {
		return nil, errors.New("not a PVR texture")
	}
	pf, dt := b[p+8], b[p+9]
	w := int(binary.LittleEndian.Uint16(b[p+12:]))
	h := int(binary.LittleEndian.Uint16(b[p+14:]))
	data := b[p+16:]
	if w <= 0 || h <= 0 || w > 1024 || h > 1024 || pf > 2 {
		return nil, errors.New("unsupported PVR format")
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	get := func(i int) uint16 {
		if i*2+1 >= len(data) {
			return 0
		}
		return binary.LittleEndian.Uint16(data[i*2:])
	}
	switch dt {
	case 1, 2, 0x0D: // twiddled (square, square+mip, rectangle)
		base := 0
		if dt == 2 {
			base = 3 // 6 byte pad
			for s := 1; s < w; s *= 2 {
				base += s * s
			}
		}
		m := w
		if h < m {
			m = h
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				i := twiddleIdx(x%m, y%m) + (x/m+y/m)*m*m
				img.SetNRGBA(x, y, texel(get(base+i), pf))
			}
		}
	case 9, 0x0B: // rectangle, row major
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.SetNRGBA(x, y, texel(get(y*w+x), pf))
			}
		}
	case 3, 4: // VQ: 256 entry codebook of 2x2 blocks, then twiddled indices
		if len(data) < 2048 {
			return nil, errors.New("short VQ texture")
		}
		cb := data[:2048]
		idx := data[2048:]
		off := 0
		if dt == 4 {
			off = 1
			for s := 2; s < w; s *= 2 {
				off += (s / 2) * (s / 2)
			}
		}
		for by := 0; by < h/2; by++ {
			for bx := 0; bx < w/2; bx++ {
				ii := off + twiddleIdx(bx, by)
				if ii >= len(idx) {
					continue
				}
				e := int(idx[ii]) * 8
				for k := 0; k < 4; k++ {
					v := binary.LittleEndian.Uint16(cb[e+k*2:])
					img.SetNRGBA(bx*2+(k>>1), by*2+(k&1), texel(v, pf))
				}
			}
		}
	default:
		return nil, errors.New("unsupported PVR layout")
	}
	return img, nil
}

// encodePVR565 makes a square twiddled RGB565 texture with GBIX + PVRT headers (32 bytes), the layout
// openMenu's BOX.DAT / ICON.DAT loader expects.
func encodePVR565(img image.Image, size int) []byte {
	src := fitSquare(img, size)
	data := make([]byte, size*size*2)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			c := src.NRGBAAt(x, y)
			// flatten transparency onto black
			r := uint32(c.R) * uint32(c.A) / 255
			g := uint32(c.G) * uint32(c.A) / 255
			bl := uint32(c.B) * uint32(c.A) / 255
			v := uint16((r>>3)<<11 | (g>>2)<<5 | bl>>3)
			binary.LittleEndian.PutUint16(data[twiddleIdx(x, y)*2:], v)
		}
	}
	hdr := make([]byte, 32)
	copy(hdr, "GBIX")
	binary.LittleEndian.PutUint32(hdr[4:], 8)
	copy(hdr[16:], "PVRT")
	binary.LittleEndian.PutUint32(hdr[20:], uint32(len(data)+8))
	hdr[24] = 1 // RGB565
	hdr[25] = 1 // square twiddled
	binary.LittleEndian.PutUint16(hdr[28:], uint16(size))
	binary.LittleEndian.PutUint16(hdr[30:], uint16(size))
	return append(hdr, data...)
}

// fitSquare centre crops to a square and resamples with a box filter (down) or bilinear (up).
func fitSquare(img image.Image, size int) *image.NRGBA {
	b := img.Bounds()
	s := b.Dx()
	if b.Dy() < s {
		s = b.Dy()
	}
	x0 := b.Min.X + (b.Dx()-s)/2
	y0 := b.Min.Y + (b.Dy()-s)/2
	return resample(img, image.Rect(x0, y0, x0+s, y0+s), size, size)
}

func resample(img image.Image, r image.Rectangle, w, h int) *image.NRGBA {
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	sx := float64(r.Dx()) / float64(w)
	sy := float64(r.Dy()) / float64(h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx0, fy0 := float64(r.Min.X)+float64(x)*sx, float64(r.Min.Y)+float64(y)*sy
			fx1, fy1 := fx0+sx, fy0+sy
			var tr, tg, tb, ta, n float64
			for yy := int(fy0); yy < int(fy1+0.999) && yy < r.Max.Y; yy++ {
				for xx := int(fx0); xx < int(fx1+0.999) && xx < r.Max.X; xx++ {
					c := color.NRGBAModel.Convert(img.At(xx, yy)).(color.NRGBA)
					tr += float64(c.R)
					tg += float64(c.G)
					tb += float64(c.B)
					ta += float64(c.A)
					n++
				}
			}
			if n == 0 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(tr / n), uint8(tg / n), uint8(tb / n), uint8(ta / n)})
		}
	}
	return out
}

// ---------- VMU screen (48 x 32, 1 bit) ----------

const vmuBytes = 192

// makeVMU turns an image into a 48x32 one bit VMU screen (row major, MSB = left pixel, 1 = dark).
// A 3:2 band is cropped from the top (box art: title logos sit at the top) or the centre (disc art:
// the logo runs across the middle), contrast stretched, then split with an Otsu threshold. Busy art
// dithers into noise at this size, so a clean threshold reads better on the real screen.
func makeVMU(img image.Image, centre bool) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	cw, ch := w, w*2/3
	if ch > h {
		ch, cw = h, h*3/2
	}
	x0 := b.Min.X + (w-cw)/2
	y0 := b.Min.Y
	if centre {
		y0 = b.Min.Y + (h-ch)/2
	}
	small := resample(img, image.Rect(x0, y0, x0+cw, y0+ch), 48, 32)
	lum := make([]float64, 48*32)
	lo, hi := 255.0, 0.0
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			c := small.NRGBAAt(x, y)
			a := float64(c.A) / 255
			l := (0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B))*a + 255*(1-a)
			lum[y*48+x] = l
			if l < lo {
				lo = l
			}
			if l > hi {
				hi = l
			}
		}
	}
	var hist [256]float64
	for i := range lum {
		if hi-lo > 1 {
			lum[i] = (lum[i] - lo) * 255 / (hi - lo)
		}
		hist[clamp(int(lum[i]), 0, 255)]++
	}
	// Otsu threshold
	total, sum := 0.0, 0.0
	for i, v := range hist {
		total += v
		sum += float64(i) * v
	}
	best, th, w0, s0 := -1.0, 128, 0.0, 0.0
	for k := 0; k < 256; k++ {
		w0 += hist[k]
		s0 += float64(k) * hist[k]
		w1 := total - w0
		if w0 == 0 || w1 == 0 {
			continue
		}
		m0, m1 := s0/w0, (sum-s0)/w1
		if v := w0 * w1 * (m0 - m1) * (m0 - m1); v > best {
			best, th = v, k
		}
	}
	out := make([]byte, vmuBytes)
	dark := 0
	for _, l := range lum {
		if int(l) <= th {
			dark++
		}
	}
	invert := dark > 48*32*6/10 // mostly dark art: draw the light parts instead so the screen is not a black slab
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			on := int(lum[y*48+x]) <= th
			if invert {
				on = !on
			}
			if on {
				out[y*6+x/8] |= 0x80 >> (x % 8)
			}
		}
	}
	return out
}

// vmuPNG renders a VMU bitmap at 4x in VMU colours for the app preview.
func vmuPNG(bits []byte) []byte {
	const s = 4
	img := image.NewNRGBA(image.Rect(0, 0, 48*s, 32*s))
	ink := color.NRGBA{29, 42, 27, 255}
	bg := color.NRGBA{159, 184, 154, 255}
	for y := 0; y < 32; y++ {
		for x := 0; x < 48; x++ {
			c := bg
			if len(bits) == vmuBytes && bits[y*6+x/8]&(0x80>>(x%8)) != 0 {
				c = ink
			}
			for dy := 0; dy < s; dy++ {
				for dx := 0; dx < s; dx++ {
					img.SetNRGBA(x*s+dx, y*s+dy, c)
				}
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func pngBytes(img image.Image) []byte {
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
