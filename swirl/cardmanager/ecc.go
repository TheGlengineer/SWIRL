package main

// EDC and ECC for raw 2352 byte mode 1 sectors (the layout in Redump style .bin tracks). A patched sector is
// written back with fresh check bytes so a drive that verifies them, or a tool that does, still accepts it.

import "encoding/binary"

var edcTable [256]uint32
var eccF, eccB [256]byte

func init() {
	for i := 0; i < 256; i++ {
		edc := uint32(i)
		for j := 0; j < 8; j++ {
			if edc&1 != 0 {
				edc = (edc >> 1) ^ 0xD8018001
			} else {
				edc >>= 1
			}
		}
		edcTable[i] = edc
		f := byte(i << 1)
		if i&0x80 != 0 {
			f ^= 0x1D
		}
		eccF[i] = f
		eccB[byte(i)^f] = byte(i)
	}
}

func edcCompute(b []byte) uint32 {
	var edc uint32
	for _, c := range b {
		edc = (edc >> 8) ^ edcTable[(edc^uint32(c))&0xFF]
	}
	return edc
}

func eccCompute(src []byte, majorCount, minorCount, majorMult, minorInc int, dest []byte) {
	size := majorCount * minorCount
	for major := 0; major < majorCount; major++ {
		index := (major>>1)*majorMult + (major & 1)
		var a, b byte
		for minor := 0; minor < minorCount; minor++ {
			t := src[index]
			index += minorInc
			if index >= size {
				index -= size
			}
			a ^= t
			b ^= t
			a = eccF[a]
		}
		a = eccB[eccF[a]^b]
		dest[major] = a
		dest[major+majorCount] = a ^ b
	}
}

// sectorIsMode1 reports whether a raw sector carries the sync pattern and a mode 1 header.
func sectorIsMode1(s []byte) bool {
	if len(s) != 2352 || s[0] != 0 || s[11] != 0 {
		return false
	}
	for i := 1; i < 11; i++ {
		if s[i] != 0xFF {
			return false
		}
	}
	return s[15] == 1
}

// sectorFixMode1 recomputes the EDC and ECC of a raw mode 1 sector in place.
func sectorFixMode1(s []byte) {
	binary.LittleEndian.PutUint32(s[2064:], edcCompute(s[:2064]))
	for i := 2068; i < 2076; i++ {
		s[i] = 0
	}
	eccCompute(s[12:], 86, 24, 2, 86, s[2076:])
	eccCompute(s[12:], 52, 43, 86, 88, s[2248:])
}
