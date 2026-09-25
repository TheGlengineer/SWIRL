package main

// Reading files out of game images: GDI (via the track table) and single file images such as CDI,
// where the sector size and LBA base are worked out from the IP.BIN and ISO9660 structures.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type rawDisc struct {
	f        *os.File
	ipOffset int64 // file offset of the user data of the IP.BIN sector (session start)
	ss       int64 // bytes per sector in the file
	baseLBA  int   // LBA of the session start
}

func (r *rawDisc) Close()                { r.f.Close() }
func (r *rawDisc) highDensityStart() int { return r.baseLBA }

func (r *rawDisc) readSectors(lba, count int) ([]byte, error) {
	out := make([]byte, 0, count*sectorSize)
	buf := make([]byte, sectorSize)
	for i := 0; i < count; i++ {
		off := r.ipOffset + int64(lba+i-r.baseLBA)*r.ss
		if off < 0 {
			return nil, errors.New("sector before session start")
		}
		if _, err := r.f.ReadAt(buf, off); err != nil {
			return nil, err
		}
		out = append(out, buf...)
	}
	return out, nil
}

func findIPOffset(f *os.File) (int64, error) {
	sig := []byte("SEGA SEGAKATANA ")
	st, err := f.Stat()
	if err != nil {
		return 0, err
	}
	limit := st.Size()
	const chunk = 1 << 20
	buf := make([]byte, chunk+64)
	for off := int64(0); off < limit; {
		n, _ := f.ReadAt(buf, off)
		if n <= 0 {
			break
		}
		if i := bytes.Index(buf[:n], sig); i >= 0 {
			return off + int64(i), nil
		}
		off += chunk
		if off == 64<<20 && limit > 128<<20 {
			off = limit - 64<<20
		}
	}
	return 0, errors.New("IP.BIN not found")
}

func openRawDisc(path string) (*rawDisc, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	ipOff, err := findIPOffset(f)
	if err != nil {
		f.Close()
		return nil, err
	}
	sec := make([]byte, sectorSize)
	for _, ss := range []int64{2048, 2336, 2352} {
		if _, err := f.ReadAt(sec, ipOff+16*ss); err != nil {
			continue
		}
		if !bytes.Equal(sec[:6], []byte("\x01CD001")) {
			continue
		}
		rootExt := int(binary.LittleEndian.Uint32(sec[158:]))
		// the root directory's "." record points at itself, which pins the LBA base
		for k := int64(17); k < 4000; k++ {
			if _, err := f.ReadAt(sec, ipOff+k*ss); err != nil {
				break
			}
			if sec[0] == 34 && sec[32] == 1 && sec[33] == 0 && int(binary.LittleEndian.Uint32(sec[2:])) == rootExt {
				return &rawDisc{f: f, ipOffset: ipOff, ss: ss, baseLBA: rootExt - int(k)}, nil
			}
		}
	}
	f.Close()
	return nil, errors.New("no readable ISO9660 file system in image")
}

// openGameDisc opens the disc image found in a game folder.
func openGameDisc(folder string) (sectorReader, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, err
	}
	var other string
	for _, e := range entries {
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".gdi" {
			return openGDI(filepath.Join(folder, e.Name()))
		}
		if ext == ".cdi" || ext == ".mdf" || ext == ".img" || ext == ".iso" {
			other = filepath.Join(folder, e.Name())
		}
	}
	if other == "" {
		return nil, errors.New("no disc image")
	}
	return openRawDisc(other)
}

// readDiscFile returns a file from the root of the game's disc (for example 0GDTEX.PVR).
func readDiscFile(folder, name string, maxSize int) ([]byte, error) {
	d, err := openGameDisc(folder)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	files, err := listISORoot(d)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		if strings.EqualFold(f.Path, name) {
			if f.Size > maxSize {
				return nil, errors.New("file too large")
			}
			b, err := d.readSectors(f.LBA, (f.Size+sectorSize-1)/sectorSize)
			if err != nil {
				return nil, err
			}
			return b[:f.Size], nil
		}
	}
	return nil, os.ErrNotExist
}

// listISORoot lists only the root directory (fast for big games).
func listISORoot(d sectorReader) ([]isoFile, error) {
	base := d.highDensityStart()
	pvd, err := d.readSectors(base+16, 1)
	if err != nil {
		return nil, err
	}
	if string(pvd[1:6]) != "CD001" {
		return nil, errors.New("no ISO9660 volume")
	}
	ext := int(binary.LittleEndian.Uint32(pvd[158:]))
	size := int(binary.LittleEndian.Uint32(pvd[166:]))
	if size > 64*sectorSize {
		size = 64 * sectorSize
	}
	data, err := d.readSectors(ext, (size+sectorSize-1)/sectorSize)
	if err != nil {
		return nil, err
	}
	var out []isoFile
	for pos := 0; pos < size; {
		ln := int(data[pos])
		if ln == 0 {
			pos = (pos/sectorSize + 1) * sectorSize
			continue
		}
		rec := data[pos : pos+ln]
		pos += ln
		nl := int(rec[32])
		name := string(rec[33 : 33+nl])
		if name == "\x00" || name == "\x01" {
			continue
		}
		if i := strings.IndexByte(name, ';'); i >= 0 {
			name = name[:i]
		}
		out = append(out, isoFile{Path: strings.TrimSuffix(name, "."), LBA: int(binary.LittleEndian.Uint32(rec[2:])),
			Size: int(binary.LittleEndian.Uint32(rec[10:])), Dir: rec[25]&2 != 0})
	}
	return out, nil
}
