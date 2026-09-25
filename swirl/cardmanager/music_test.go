package main

import (
	"math"
	"testing"
)

type aicaDec struct{ pred, step int }

func (d *aicaDec) dec(n int) int {
	if d.step == 0 {
		d.step = 127
	}
	d.pred += d.step * aicaDiff[n] / 8
	if d.pred > 32767 {
		d.pred = 32767
	} else if d.pred < -32768 {
		d.pred = -32768
	}
	d.step = d.step * aicaScale[n] >> 8
	if d.step < 127 {
		d.step = 127
	} else if d.step > 24576 {
		d.step = 24576
	}
	return d.pred
}

func TestBGMLoopsCleanly(t *testing.T) {
	p := &pcm{rate: 22050, channels: 2}
	for i := 0; i < 22050*3; i++ {
		v := float32(0.3 * math.Sin(float64(i)*2*math.Pi*440/22050))
		w := float32(0.25 * math.Sin(float64(i)*2*math.Pi*660/22050))
		p.samples = append(p.samples, v, w)
	}
	out, _, err := encodeBGM(p)
	if err != nil {
		t.Fatal(err)
	}
	payload := out[32:]
	if len(payload)%32 != 0 {
		t.Fatal("not padded")
	}
	var l, r aicaDec
	for pass := 0; pass < 3; pass++ {
		for _, b := range payload {
			l.dec(int(b >> 4))
			r.dec(int(b & 15))
		}
		if l.pred != 0 || r.pred != 0 || l.step != 127 || r.step != 127 {
			t.Fatalf("after pass %d the decoder is at %+v %+v, not the start state", pass, l, r)
		}
	}
}
