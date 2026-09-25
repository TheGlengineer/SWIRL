package main

// The picture SWIRL shows on the VMU at power on and on its System screens. The owner can turn any
// image into one here; it is written to the menu disc as LOGO.VMU.

import (
	_ "embed"
	"errors"
	"image"
	"image/color"
	"os"
	"path/filepath"
)

//go:embed assets/default_logo.bin
var defaultVMULogo []byte

func logoPath(root string) string { return filepath.Join(root, editsDir, "logo_vmu.bin") }

type LogoOptions struct {
	Fit       string `json:"fit"`       // "crop" (fill the screen) or "fit" (whole picture, with borders)
	Threshold int    `json:"threshold"` // 0..255, or -1 to pick automatically
	Invert    bool   `json:"invert"`
	Dither    bool   `json:"dither"`
	Zoom      int    `json:"zoom"` // 100..300 percent, crop mode
	OffsetX   int    `json:"offsetX"`
	OffsetY   int    `json:"offsetY"` // -100..100, where to crop
}

// makeLogo converts a picture to a 48x32 one bit VMU screen (row major, MSB left, 1 = dark).
func makeLogo(img image.Image, o LogoOptions) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	// area of the source to use, 3:2
	var src image.Rectangle
	if o.Fit == "fit" {
		src = b
	} else {
		zoom := o.Zoom
		if zoom < 100 {
			zoom = 100
		}
		cw, ch := w, w*2/3
		if ch > h {
			ch, cw = h, h*3/2
		}
		cw, ch = cw*100/zoom, ch*100/zoom
		if cw < 1 {
			cw = 1
		}
		if ch < 1 {
			ch = 1
		}
		x0 := b.Min.X + (w-cw)/2 + (w-cw)/2*o.OffsetX/100
		y0 := b.Min.Y + (h-ch)/2 + (h-ch)/2*o.OffsetY/100
		src = image.Rect(x0, y0, x0+cw, y0+ch)
	}
	// target area inside 48x32 (fit mode keeps the aspect ratio)
	tw, th := 48, 32
	if o.Fit == "fit" {
		if w*32 > h*48 {
			th = h * 48 / w
		} else {
			tw = w * 32 / h
		}
		if tw < 1 {
			tw = 1
		}
		if th < 1 {
			th = 1
		}
	}
	small := resample(img, src, tw, th)
	lum := make([]float64, 48*32)
	for i := range lum {
		lum[i] = 255 // blank border is light (no ink)
	}
	ox, oy := (48-tw)/2, (32-th)/2
	minL, maxL := 255.0, 0.0
	for y := 0; y < th; y++ {
		for x := 0; x < tw; x++ {
			c := color.NRGBAModel.Convert(small.At(x, y)).(color.NRGBA)
			a := float64(c.A) / 255
			l := (0.299*float64(c.R)+0.587*float64(c.G)+0.114*float64(c.B))*a + 255*(1-a)
			lum[(y+oy)*48+x+ox] = l
			if l < minL {
				minL = l
			}
			if l > maxL {
				maxL = l
			}
		}
	}
	// stretch contrast so dim photos still split into ink and paper
	if maxL-minL > 8 {
		for i, l := range lum {
			lum[i] = (l - minL) * 255 / (maxL - minL)
			if lum[i] > 255 {
				lum[i] = 255
			}
		}
	}
	if o.Invert {
		for i := range lum {
			lum[i] = 255 - lum[i]
		}
	}
	th2 := float64(o.Threshold)
	if o.Threshold < 0 || o.Threshold > 255 {
		th2 = otsu(lum)
	}
	out := make([]byte, vmuBytes)
	if o.Dither {
		buf := append([]float64(nil), lum...)
		for y := 0; y < 32; y++ {
			for x := 0; x < 48; x++ {
				i := y*48 + x
				old := buf[i]
				nv := 255.0
				if old < th2 {
					nv = 0
					out[y*6+x/8] |= 0x80 >> (x % 8)
				}
				e := old - nv
				spread := func(dx, dy int, f float64) {
					xx, yy := x+dx, y+dy
					if xx >= 0 && xx < 48 && yy < 32 {
						buf[yy*48+xx] += e * f
					}
				}
				spread(1, 0, 7.0/16)
				spread(-1, 1, 3.0/16)
				spread(0, 1, 5.0/16)
				spread(1, 1, 1.0/16)
			}
		}
		return out
	}
	for i, l := range lum {
		if l < th2 {
			out[(i/48)*6+(i%48)/8] |= 0x80 >> ((i % 48) % 8)
		}
	}
	return out
}

func otsu(lum []float64) float64 {
	var hist [256]float64
	for _, l := range lum {
		v := int(l)
		if v < 0 {
			v = 0
		}
		if v > 255 {
			v = 255
		}
		hist[v]++
	}
	total := float64(len(lum))
	var sum float64
	for i := 0; i < 256; i++ {
		sum += float64(i) * hist[i]
	}
	var sumB, wB, best float64
	th := 128.0
	for t := 0; t < 256; t++ {
		wB += hist[t]
		if wB == 0 {
			continue
		}
		wF := total - wB
		if wF == 0 {
			break
		}
		sumB += float64(t) * hist[t]
		mB, mF := sumB/wB, (sum-sumB)/wF
		v := wB * wF * (mB - mF) * (mB - mF)
		if v > best {
			best, th = v, float64(t)+0.5
		}
	}
	return th
}

func GetLogo(root string) ([]byte, bool) {
	if b, err := os.ReadFile(logoPath(root)); err == nil && len(b) == vmuBytes {
		return b, true
	}
	return defaultVMULogo, false
}

func SaveLogo(root string, bits []byte) error {
	if bits == nil {
		err := os.Remove(logoPath(root))
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(bits) != vmuBytes {
		return errors.New("a VMU logo is 192 bytes")
	}
	os.MkdirAll(filepath.Join(root, editsDir), 0o755)
	return os.WriteFile(logoPath(root), bits, 0o644)
}
