package main

// The header logo sheet, HDRLOGO.PVR, on the menu disc (F1, SW-15).
//
// SWIRL's header shows the Sega Dreamcast logo cut from openMenu's USA theme picture (BG_U_L.PVR, 512 KB).
// Reading that whole picture at every boot cost about half a second, and a European console showed the orange
// American swirl. Card Manager now cuts the logo out once, at Update SWIRL, and writes a 128 x 128 texture
// (32 KB) holding two copies: the orange one for USA and Japan in the top band, and a blue one for Europe in
// the band below, made from the same pixels with the swirl's hue turned to the PAL blue. The menu reads this
// small file when it is there and picks a band by the console's region; on a card written before 2.15 it
// reads the big picture as before.

import (
	"errors"
	"image"
	"image/color"
	"math"
)

const (
	hdrLogoOnDisc = "HDRLOGO.PVR"
	hdrLogoSize   = 128 // the texture is square
	hdrLogoW      = 114 // the logo inside openMenu's picture
	hdrLogoH      = 20
	hdrLogoSrcX   = 4
	hdrLogoSrcY   = 34
	hdrLogoBand   = 32 // rows per band in the sheet: 0 orange, 32 blue
)

// hdrLogoBlue turns the orange swirl blue: the same hue for every swirl pixel, saturation and brightness kept
// (a touch deeper), so the swirl's own shading survives. Text and plate are grey and white and are left alone.
func hdrLogoBlue(c color.NRGBA) color.NRGBA {
	_, s, v := rgbToHSV(c)
	if s <= 0.18 || v <= 0.15 {
		return c
	}
	r, g, b := hsvToRGB(207.0/360.0, math.Min(1, s*1.05), math.Min(1, v*0.82))
	return color.NRGBA{r, g, b, c.A}
}

func rgbToHSV(c color.NRGBA) (h, s, v float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	max := math.Max(r, math.Max(g, b))
	min := math.Min(r, math.Min(g, b))
	v = max
	d := max - min
	if max > 0 {
		s = d / max
	}
	if d == 0 {
		return 0, s, v
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h / 6, s, v
}

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	i := math.Floor(h * 6)
	f := h*6 - i
	p, q, t := v*(1-s), v*(1-f*s), v*(1-(1-f)*s)
	var r, g, b float64
	switch int(i) % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return uint8(r*255 + 0.5), uint8(g*255 + 0.5), uint8(b*255 + 0.5)
}

// buildHeaderLogoSheet makes HDRLOGO.PVR from openMenu's USA theme picture.
func buildHeaderLogoSheet(bgUL []byte) ([]byte, error) {
	src, err := decodePVR(bgUL)
	if err != nil {
		return nil, err
	}
	if b := src.Bounds(); b.Dx() < hdrLogoSrcX+hdrLogoW || b.Dy() < hdrLogoSrcY+hdrLogoH {
		return nil, errors.New("the theme picture is smaller than expected")
	}
	sheet := image.NewNRGBA(image.Rect(0, 0, hdrLogoSize, hdrLogoSize))
	// white everywhere: the plate colour, so nothing stray shows at the band edges
	for i := range sheet.Pix {
		sheet.Pix[i] = 255
	}
	for y := 0; y < hdrLogoH; y++ {
		for x := 0; x < hdrLogoW; x++ {
			c := src.NRGBAAt(hdrLogoSrcX+x, hdrLogoSrcY+y)
			sheet.SetNRGBA(x, y, c)
			sheet.SetNRGBA(x, hdrLogoBand+y, hdrLogoBlue(c))
		}
	}
	return encodePVR565(sheet, hdrLogoSize), nil
}

// addHeaderLogoSheet writes HDRLOGO.PVR into the menu disc's files. It is rebuilt at every Update SWIRL from
// the cached theme picture, so a card gets the current sheet whatever it had before.
func addHeaderLogoSheet(data string, log Logger) {
	bg, err := fetchHeaderLogo()
	if err != nil {
		return // addHeaderLogo has already said the logo is missing
	}
	sheet, err := buildHeaderLogoSheet(bg)
	if err != nil {
		log("The header logo sheet was not made (%v); the menu reads the theme picture instead", err)
		return
	}
	if err := placeOnDisc(data, hdrLogoOnDisc, sheet); err != nil {
		log("The header logo sheet could not be added: %v", err)
	}
}
