package main

// Fresh SD card layout: an MBR with one FAT32 partition, formatted from scratch.
// Windows will not format cards over 32 GB as FAT32, so SWIRL writes the file system itself.

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"
)

const (
	secSize       = 512
	partStartLBA  = 2048 // 1 MiB, the usual SD card alignment
	minFAT32Clus  = 65525
	maxMBRSectors = 0xFFFFFFFF
)

type fat32Layout struct {
	DiskSectors  uint64
	PartStart    uint32
	PartSectors  uint32
	SecPerClus   uint32
	Reserved     uint32
	FATSectors   uint32
	Clusters     uint32
	DataStartLBA uint64 // absolute, on the disk
	VolID        uint32
	Label        string
}

func (l *fat32Layout) ClusterBytes() int { return int(l.SecPerClus) * secSize }

// planFAT32 picks the partition and FAT32 geometry for a disk of the given size.
// Clusters are 32 KB when the card is big enough, which is what GDEMU cards are usually formatted with.
func planFAT32(diskSectors uint64, label string) (*fat32Layout, error) {
	if diskSectors < partStartLBA+2*1024*64 {
		return nil, errors.New("the card is too small for FAT32")
	}
	if diskSectors-partStartLBA > maxMBRSectors {
		return nil, errors.New("cards larger than 2 TB are not supported")
	}
	l := &fat32Layout{DiskSectors: diskSectors, PartStart: partStartLBA, PartSectors: uint32(diskSectors - partStartLBA), Label: label}
	for spc := uint32(64); spc >= 1; spc /= 2 {
		l.SecPerClus = spc
		l.Reserved = 32
		// Microsoft's FAT32 sizing formula
		tmp1 := uint64(l.PartSectors - l.Reserved)
		tmp2 := uint64(256*spc+2) / 2
		l.FATSectors = uint32((tmp1 + tmp2 - 1) / tmp2)
		// grow the reserved area so the data region starts on a 1 MiB boundary
		dataStart := uint64(l.PartStart) + uint64(l.Reserved) + 2*uint64(l.FATSectors)
		if pad := (2048 - dataStart%2048) % 2048; pad > 0 {
			l.Reserved += uint32(pad)
		}
		l.DataStartLBA = uint64(l.PartStart) + uint64(l.Reserved) + 2*uint64(l.FATSectors)
		l.Clusters = uint32((uint64(l.PartSectors) - uint64(l.Reserved) - 2*uint64(l.FATSectors)) / uint64(spc))
		if l.Clusters >= minFAT32Clus || spc == 1 {
			break
		}
	}
	if l.Clusters < minFAT32Clus {
		return nil, errors.New("the card is too small for FAT32")
	}
	if uint64(l.FATSectors)*secSize/4 < uint64(l.Clusters)+2 {
		return nil, errors.New("internal error: FAT too small")
	}
	var b [4]byte
	rand.Read(b[:])
	l.VolID = binary.LittleEndian.Uint32(b[:])
	return l, nil
}

func chs(lba uint64) [3]byte {
	const heads, spt = 255, 63
	c := lba / (heads * spt)
	if c > 1023 {
		return [3]byte{0xFE, 0xFF, 0xFF}
	}
	h := (lba / spt) % heads
	s := lba%spt + 1
	return [3]byte{byte(h), byte(s) | byte((c>>2)&0xC0), byte(c)}
}

func (l *fat32Layout) mbr() []byte {
	b := make([]byte, secSize)
	rand.Read(b[440:444]) // disk signature
	p := b[446:]
	p[0] = 0x00
	s := chs(uint64(l.PartStart))
	copy(p[1:4], s[:])
	p[4] = 0x0C // FAT32 with LBA
	e := chs(uint64(l.PartStart) + uint64(l.PartSectors) - 1)
	copy(p[5:8], e[:])
	binary.LittleEndian.PutUint32(p[8:], l.PartStart)
	binary.LittleEndian.PutUint32(p[12:], l.PartSectors)
	b[510], b[511] = 0x55, 0xAA
	return b
}

func padLabel(s string) []byte {
	out := []byte("           ")
	for i := 0; i < len(s) && i < 11; i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32
		}
		out[i] = c
	}
	return out
}

func (l *fat32Layout) bootSector() []byte {
	b := make([]byte, secSize)
	copy(b[0:], []byte{0xEB, 0x58, 0x90})
	copy(b[3:], "MSWIN4.1")
	binary.LittleEndian.PutUint16(b[11:], secSize)
	b[13] = byte(l.SecPerClus)
	binary.LittleEndian.PutUint16(b[14:], uint16(l.Reserved))
	b[16] = 2 // FAT copies
	b[21] = 0xF8
	binary.LittleEndian.PutUint16(b[24:], 63)
	binary.LittleEndian.PutUint16(b[26:], 255)
	binary.LittleEndian.PutUint32(b[28:], l.PartStart)
	binary.LittleEndian.PutUint32(b[32:], l.PartSectors)
	binary.LittleEndian.PutUint32(b[36:], l.FATSectors)
	binary.LittleEndian.PutUint32(b[44:], 2) // root directory cluster
	binary.LittleEndian.PutUint16(b[48:], 1) // FSInfo sector
	binary.LittleEndian.PutUint16(b[50:], 6) // backup boot sector
	b[64] = 0x80
	b[66] = 0x29
	binary.LittleEndian.PutUint32(b[67:], l.VolID)
	copy(b[71:], padLabel(l.Label))
	copy(b[82:], "FAT32   ")
	// not bootable: halt if a PC ever tries
	copy(b[90:], []byte{0xFA, 0xF4, 0xEB, 0xFD})
	b[510], b[511] = 0x55, 0xAA
	return b
}

func (l *fat32Layout) fsInfo() []byte {
	b := make([]byte, secSize)
	binary.LittleEndian.PutUint32(b[0:], 0x41615252)
	binary.LittleEndian.PutUint32(b[484:], 0x61417272)
	binary.LittleEndian.PutUint32(b[488:], l.Clusters-1) // cluster 2 is the root directory
	binary.LittleEndian.PutUint32(b[492:], 3)
	binary.LittleEndian.PutUint32(b[508:], 0xAA550000)
	return b
}

func fatTimestamp(t time.Time) (uint16, uint16) {
	d := uint16((t.Year()-1980)<<9 | int(t.Month())<<5 | t.Day())
	tm := uint16(t.Hour()<<11 | t.Minute()<<5 | t.Second()/2)
	return d, tm
}

// writeFreshCard writes a new MBR and an empty FAT32 file system to a whole disk.
// The MBR is written last so an interrupted run leaves no half-valid partition.
func writeFreshCard(w io.WriterAt, l *fat32Layout, progress func(done, total uint64)) error {
	const chunk = 1 << 20
	zero := make([]byte, chunk)
	var total, done uint64
	fatBytes := uint64(l.FATSectors) * secSize
	total = 2*partStartLBA*secSize + uint64(l.Reserved)*secSize + 2*fatBytes + uint64(l.ClusterBytes())
	step := func(n int) {
		done += uint64(n)
		if progress != nil {
			progress(done, total)
		}
	}
	zeroRange := func(off, n uint64) error {
		for n > 0 {
			k := uint64(chunk)
			if n < k {
				k = n
			}
			if _, err := w.WriteAt(zero[:k], int64(off)); err != nil {
				return fmt.Errorf("writing to the card at %d MB: %w", off>>20, err)
			}
			off += k
			n -= k
			step(int(k))
		}
		return nil
	}
	diskBytes := l.DiskSectors * secSize
	// wipe old partition tables: the start (sector 0 comes later) and the last MiB (backup GPT)
	if err := zeroRange(secSize, partStartLBA*secSize-secSize); err != nil {
		return err
	}
	if err := zeroRange(diskBytes-partStartLBA*secSize, partStartLBA*secSize); err != nil {
		return err
	}
	base := uint64(l.PartStart) * secSize
	// reserved area, FATs and the root directory cluster
	if err := zeroRange(base, uint64(l.Reserved)*secSize); err != nil {
		return err
	}
	if err := zeroRange(base+uint64(l.Reserved)*secSize, 2*fatBytes); err != nil {
		return err
	}
	root := make([]byte, l.ClusterBytes())
	copy(root[0:11], padLabel(l.Label))
	root[11] = 0x08 // volume label
	d, t := fatTimestamp(time.Now())
	binary.LittleEndian.PutUint16(root[22:], t)
	binary.LittleEndian.PutUint16(root[24:], d)
	if _, err := w.WriteAt(root, int64(l.DataStartLBA*secSize)); err != nil {
		return err
	}
	step(len(root))
	fat := make([]byte, secSize)
	binary.LittleEndian.PutUint32(fat[0:], 0x0FFFFFF8)
	binary.LittleEndian.PutUint32(fat[4:], 0x0FFFFFFF)
	binary.LittleEndian.PutUint32(fat[8:], 0x0FFFFFFF) // root directory, one cluster
	for i := uint64(0); i < 2; i++ {
		if _, err := w.WriteAt(fat, int64(base+uint64(l.Reserved)*secSize+i*fatBytes)); err != nil {
			return err
		}
	}
	boot, info := l.bootSector(), l.fsInfo()
	sig := make([]byte, secSize)
	sig[510], sig[511] = 0x55, 0xAA
	for _, s := range []struct {
		sec uint64
		b   []byte
	}{{6, boot}, {7, info}, {8, sig}, {2, sig}, {1, info}, {0, boot}} {
		if _, err := w.WriteAt(s.b, int64(base+s.sec*secSize)); err != nil {
			return err
		}
	}
	if _, err := w.WriteAt(l.mbr(), 0); err != nil {
		return err
	}
	return nil
}

// gdemuINI is written to a fresh card. reset_goto = 1 makes the GDEMU button return to folder 01 (SWIRL).
const gdemuINI = "reset_goto = 1\r\n"
