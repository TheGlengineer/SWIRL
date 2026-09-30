package main

// A VCDIFF (RFC 3284) decoder for the xdelta patches inside .dcp files. Only what those patches use: the
// default code table, no secondary compression, and xdelta3's Adler32 window checksum, which is checked so a
// patch made for another version of a file is refused before anything is written.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/adler32"
)

type vcdInst struct {
	op   byte // 0 no op, 1 ADD, 2 RUN, 3 COPY
	size int
	mode int
}

var vcdTable [256][2]vcdInst

func init() {
	i := 0
	set := func(a, b vcdInst) { vcdTable[i] = [2]vcdInst{a, b}; i++ }
	set(vcdInst{op: 2}, vcdInst{})
	for s := 0; s <= 17; s++ {
		set(vcdInst{op: 1, size: s}, vcdInst{})
	}
	for m := 0; m <= 8; m++ {
		set(vcdInst{op: 3, mode: m}, vcdInst{})
		for s := 4; s <= 18; s++ {
			set(vcdInst{op: 3, size: s, mode: m}, vcdInst{})
		}
	}
	for a := 1; a <= 4; a++ {
		for m := 0; m <= 5; m++ {
			for s := 4; s <= 6; s++ {
				set(vcdInst{op: 1, size: a}, vcdInst{op: 3, size: s, mode: m})
			}
		}
	}
	for a := 1; a <= 4; a++ {
		for m := 6; m <= 8; m++ {
			set(vcdInst{op: 1, size: a}, vcdInst{op: 3, size: 4, mode: m})
		}
	}
	for m := 0; m <= 8; m++ {
		set(vcdInst{op: 3, size: 4, mode: m}, vcdInst{op: 1, size: 1})
	}
	if i != 256 {
		panic("vcdiff code table")
	}
}

type vcdReader struct {
	b   []byte
	pos int
}

func (r *vcdReader) byte_() (byte, error) {
	if r.pos >= len(r.b) {
		return 0, errors.New("patch is cut short")
	}
	c := r.b[r.pos]
	r.pos++
	return c, nil
}

func (r *vcdReader) varint() (int, error) {
	v := 0
	for n := 0; n < 5; n++ {
		c, err := r.byte_()
		if err != nil {
			return 0, err
		}
		v = (v << 7) | int(c&0x7f)
		if c&0x80 == 0 {
			return v, nil
		}
	}
	return 0, errors.New("bad number in patch")
}

func (r *vcdReader) take(n int) ([]byte, error) {
	if n < 0 || r.pos+n > len(r.b) {
		return nil, errors.New("patch is cut short")
	}
	s := r.b[r.pos : r.pos+n]
	r.pos += n
	return s, nil
}

// vcdiffApply decodes delta against source and returns the target. A window whose checksum does not match is an
// error, as is any patch feature these patches do not use.
func vcdiffApply(delta, source []byte) ([]byte, error) {
	r := &vcdReader{b: delta}
	hdr, err := r.take(4)
	if err != nil {
		return nil, err
	}
	if hdr[0] != 0xD6 || hdr[1] != 0xC3 || hdr[2] != 0xC4 || hdr[3] != 0 {
		return nil, errors.New("not an xdelta (VCDIFF) patch")
	}
	ind, err := r.byte_()
	if err != nil {
		return nil, err
	}
	secondary := false
	if ind&1 != 0 {
		if _, err := r.byte_(); err != nil {
			return nil, err
		}
		secondary = true
	}
	if ind&2 != 0 {
		return nil, errors.New("patch uses a custom code table, which is not supported")
	}
	if ind&4 != 0 {
		n, err := r.varint()
		if err != nil {
			return nil, err
		}
		if _, err := r.take(n); err != nil {
			return nil, err
		}
	}
	var out []byte
	for r.pos < len(r.b) {
		win, err := r.byte_()
		if err != nil {
			return nil, err
		}
		var src []byte
		if win&3 != 0 {
			slen, err := r.varint()
			if err != nil {
				return nil, err
			}
			spos, err := r.varint()
			if err != nil {
				return nil, err
			}
			from := source
			if win&2 != 0 {
				from = out
			}
			if spos < 0 || slen < 0 || spos+slen > len(from) {
				return nil, errors.New("patch was made for a different (larger) file")
			}
			src = from[spos : spos+slen]
		}
		if _, err := r.varint(); err != nil { // delta encoding length
			return nil, err
		}
		tlen, err := r.varint()
		if err != nil {
			return nil, err
		}
		dind, err := r.byte_()
		if err != nil {
			return nil, err
		}
		if dind != 0 && secondary {
			return nil, errors.New("patch uses secondary compression, which is not supported")
		}
		dataLen, err := r.varint()
		if err != nil {
			return nil, err
		}
		instLen, err := r.varint()
		if err != nil {
			return nil, err
		}
		addrLen, err := r.varint()
		if err != nil {
			return nil, err
		}
		var want uint32
		check := win&4 != 0
		if check {
			b, err := r.take(4)
			if err != nil {
				return nil, err
			}
			want = binary.BigEndian.Uint32(b)
		}
		data, err := r.take(dataLen)
		if err != nil {
			return nil, err
		}
		inst, err := r.take(instLen)
		if err != nil {
			return nil, err
		}
		addr, err := r.take(addrLen)
		if err != nil {
			return nil, err
		}
		target, err := vcdiffWindow(src, tlen, data, inst, addr)
		if err != nil {
			return nil, err
		}
		if check && adler32.Checksum(target) != want {
			return nil, errors.New("the file is not the one this patch was made for")
		}
		out = append(out, target...)
	}
	return out, nil
}

func vcdiffWindow(src []byte, tlen int, data, inst, addr []byte) ([]byte, error) {
	const nearSize, sameSize = 4, 3
	var near [nearSize]int
	var same [sameSize * 256]int
	nextNear := 0
	target := make([]byte, 0, tlen)
	dr := &vcdReader{b: data}
	ir := &vcdReader{b: inst}
	ar := &vcdReader{b: addr}
	decodeAddr := func(mode int) (int, error) {
		here := len(src) + len(target)
		var a int
		var err error
		switch {
		case mode == 0:
			a, err = ar.varint()
		case mode == 1:
			a, err = ar.varint()
			a = here - a
		case mode < 2+nearSize:
			a, err = ar.varint()
			a += near[mode-2]
		default:
			var c byte
			c, err = ar.byte_()
			a = same[(mode-2-nearSize)*256+int(c)]
		}
		if err != nil {
			return 0, err
		}
		near[nextNear] = a
		nextNear = (nextNear + 1) % nearSize
		same[a%(sameSize*256)] = a
		return a, nil
	}
	for len(target) < tlen {
		code, err := ir.byte_()
		if err != nil {
			return nil, err
		}
		for _, in := range vcdTable[code] {
			if in.op == 0 {
				continue
			}
			size := in.size
			if size == 0 {
				if size, err = ir.varint(); err != nil {
					return nil, err
				}
			}
			switch in.op {
			case 1:
				b, err := dr.take(size)
				if err != nil {
					return nil, err
				}
				target = append(target, b...)
			case 2:
				c, err := dr.byte_()
				if err != nil {
					return nil, err
				}
				for i := 0; i < size; i++ {
					target = append(target, c)
				}
			case 3:
				a, err := decodeAddr(in.mode)
				if err != nil {
					return nil, err
				}
				for i := 0; i < size; i++ {
					p := a + i
					var c byte
					if p < len(src) {
						c = src[p]
					} else if p-len(src) < len(target) {
						c = target[p-len(src)]
					} else {
						return nil, fmt.Errorf("patch copies from beyond the file")
					}
					target = append(target, c)
				}
			}
		}
	}
	if len(target) != tlen {
		return nil, errors.New("patch output has the wrong length")
	}
	return target, nil
}
