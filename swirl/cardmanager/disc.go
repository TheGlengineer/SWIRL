package main

// Reading Dreamcast disc images: GDI track tables, IP.BIN headers and ISO9660 file trees.

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const sectorSize = 2048

type gdiTrack struct {
	Num, LBA, Type, SectorSize int
	File                       string
	Sectors                    int
}

func parseGDI(path string) ([]gdiTrack, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tracks []gdiTrack
	sc := bufio.NewScanner(f)
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			continue
		}
		fields := splitGDILine(line)
		if len(fields) < 5 {
			continue
		}
		num, _ := strconv.Atoi(fields[0])
		lba, _ := strconv.Atoi(fields[1])
		typ, _ := strconv.Atoi(fields[2])
		ss, _ := strconv.Atoi(fields[3])
		t := gdiTrack{Num: num, LBA: lba, Type: typ, SectorSize: ss, File: filepath.Join(filepath.Dir(path), fields[4])}
		if st, err := os.Stat(t.File); err == nil && ss > 0 {
			t.Sectors = int(st.Size()) / ss
		}
		tracks = append(tracks, t)
	}
	if len(tracks) == 0 {
		return nil, errors.New("empty GDI")
	}
	return tracks, nil
}

// splitGDILine handles quoted file names with spaces.
func splitGDILine(line string) []string {
	var out []string
	var cur strings.Builder
	quoted := false
	for _, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case (r == ' ' || r == '\t') && !quoted:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

// gdiDisc reads 2048 byte user data sectors by absolute LBA across the data tracks of a GDI.
type gdiDisc struct {
	tracks []gdiTrack
	files  map[string]*os.File
}

func openGDI(path string) (*gdiDisc, error) {
	tracks, err := parseGDI(path)
	if err != nil {
		return nil, err
	}
	d := &gdiDisc{files: map[string]*os.File{}}
	for _, t := range tracks {
		if t.Type == 4 {
			d.tracks = append(d.tracks, t)
		}
	}
	return d, nil
}

func (d *gdiDisc) Close() {
	for _, f := range d.files {
		f.Close()
	}
}

func (d *gdiDisc) highDensityStart() int {
	best := -1
	for _, t := range d.tracks {
		if t.LBA >= 45000 && (best < 0 || t.LBA < best) {
			best = t.LBA
		}
	}
	if best < 0 && len(d.tracks) > 0 {
		best = d.tracks[0].LBA
	}
	return best
}

func (d *gdiDisc) readSectors(lba, count int) ([]byte, error) {
	out := make([]byte, 0, count*sectorSize)
	buf := make([]byte, sectorSize)
	for i := 0; i < count; i++ {
		found := false
		for _, t := range d.tracks {
			if lba+i >= t.LBA && lba+i < t.LBA+t.Sectors {
				f := d.files[t.File]
				if f == nil {
					var err error
					if f, err = os.Open(t.File); err != nil {
						return nil, err
					}
					d.files[t.File] = f
				}
				off := int64(lba+i-t.LBA) * int64(t.SectorSize)
				if t.SectorSize == 2352 {
					off += 16
				}
				if _, err := f.ReadAt(buf, off); err != nil && err != io.EOF {
					return nil, err
				}
				out = append(out, buf...)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("LBA %d is not on the disc", lba+i)
		}
	}
	return out, nil
}

// ---------- IP.BIN ----------

type ipInfo struct {
	Name, Product, Version, Date, Region, Disc string
	VGA                                        bool
}

func trimField(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		b = b[:i]
	}
	return strings.TrimSpace(string(b))
}

func parseIP(b []byte) (*ipInfo, bool) {
	if len(b) < 0x100 || !bytes.HasPrefix(b, []byte("SEGA SEGAKATANA")) {
		return nil, false
	}
	ip := &ipInfo{
		Region:  trimField(b[0x30:0x38]),
		Product: trimField(b[0x40:0x4A]),
		Version: trimField(b[0x4A:0x50]),
		Date:    trimField(b[0x50:0x58]),
		Name:    trimField(b[0x80:0x100]),
		VGA:     b[0x38+5] == '1',
	}
	dn, dt := b[0x2B], b[0x2D]
	if dn == ' ' || dt == ' ' || dn < '0' || dn > '9' || dt < '0' || dt > '9' {
		ip.Disc = "1/1"
	} else {
		ip.Disc = fmt.Sprintf("%c/%c", dn, dt)
	}
	return ip, true
}

// readImageIP finds the IP.BIN of the disc image in a game folder.
func readImageIP(folder string) (*ipInfo, string, error) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return nil, "", err
	}
	var gdi, other string
	for _, e := range entries {
		if isJunk(e.Name()) { // macOS "._" files and the like are not discs
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		switch ext {
		case ".gdi":
			gdi = filepath.Join(folder, e.Name())
		case ".cdi", ".mdf", ".img", ".iso":
			if other == "" {
				other = filepath.Join(folder, e.Name())
			}
		}
	}
	if gdi != "" {
		d, err := openGDI(gdi)
		if err != nil {
			return nil, "GDI", err
		}
		defer d.Close()
		b, err := d.readSectors(d.highDensityStart(), 1)
		if err != nil {
			return nil, "GDI", err
		}
		if ip, ok := parseIP(b); ok {
			return ip, "GDI", nil
		}
		return nil, "GDI", errors.New("no IP.BIN in GDI")
	}
	if other != "" {
		format := strings.ToUpper(strings.TrimPrefix(filepath.Ext(other), "."))
		ip, err := scanForIP(other)
		return ip, format, err
	}
	return nil, "", errors.New("no disc image")
}

// scanForIP searches the first part of an image for the IP.BIN signature (CDI, MDF, IMG, ISO).
func scanForIP(path string) (*ipInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sig := []byte("SEGA SEGAKATANA ")
	const chunk = 1 << 20
	buf := make([]byte, chunk+512)
	var off int64
	st, _ := f.Stat()
	limit := st.Size()
	for off < limit {
		n, _ := f.ReadAt(buf, off)
		if n <= 0 {
			break
		}
		if i := bytes.Index(buf[:n], sig); i >= 0 {
			hdr := make([]byte, 0x100)
			f.ReadAt(hdr, off+int64(i))
			if ip, ok := parseIP(hdr); ok {
				return ip, nil
			}
		}
		off += chunk
		// CDI images keep the data session at the end: after 64 MB jump to the last 64 MB.
		if off == 64<<20 && limit > 128<<20 {
			off = limit - 64<<20
		}
	}
	return nil, errors.New("IP.BIN not found")
}

// ---------- ISO9660 reading ----------

type isoFile struct {
	Path string // relative, forward slashes, upper case as stored
	LBA  int
	Size int
	Dir  bool
}

type sectorReader interface {
	readSectors(lba, count int) ([]byte, error)
	highDensityStart() int
	Close()
}

func listISO(d sectorReader) ([]isoFile, error) {
	base := d.highDensityStart()
	pvd, err := d.readSectors(base+16, 1)
	if err != nil {
		return nil, err
	}
	if string(pvd[1:6]) != "CD001" {
		return nil, errors.New("no ISO9660 volume on the menu disc")
	}
	rootExt := int(binary.LittleEndian.Uint32(pvd[156+2:]))
	rootSize := int(binary.LittleEndian.Uint32(pvd[156+10:]))
	var out []isoFile
	var walk func(ext, size int, rel string, depth int) error
	walk = func(ext, size int, rel string, depth int) error {
		if depth > 8 {
			return nil
		}
		data, err := d.readSectors(ext, (size+sectorSize-1)/sectorSize)
		if err != nil {
			return err
		}
		pos := 0
		for pos < size {
			ln := int(data[pos])
			if ln == 0 {
				pos = (pos/sectorSize + 1) * sectorSize
				continue
			}
			rec := data[pos : pos+ln]
			pos += ln
			e := int(binary.LittleEndian.Uint32(rec[2:]))
			sz := int(binary.LittleEndian.Uint32(rec[10:]))
			flags := rec[25]
			nl := int(rec[32])
			name := string(rec[33 : 33+nl])
			if name == "\x00" || name == "\x01" {
				continue
			}
			if i := strings.IndexByte(name, ';'); i >= 0 {
				name = name[:i]
			}
			name = strings.TrimSuffix(name, ".")
			p := name
			if rel != "" {
				p = rel + "/" + name
			}
			if flags&2 != 0 {
				out = append(out, isoFile{Path: p, LBA: e, Size: sz, Dir: true})
				if err := walk(e, sz, p, depth+1); err != nil {
					return err
				}
			} else {
				out = append(out, isoFile{Path: p, LBA: e, Size: sz})
			}
		}
		return nil
	}
	if err := walk(rootExt, rootSize, "", 0); err != nil {
		return nil, err
	}
	return out, nil
}

func extractISOFile(d sectorReader, f isoFile, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	remaining := f.Size
	lba := f.LBA
	for remaining > 0 {
		n := 256
		if need := (remaining + sectorSize - 1) / sectorSize; need < n {
			n = need
		}
		b, err := d.readSectors(lba, n)
		if err != nil {
			return err
		}
		if len(b) > remaining {
			b = b[:remaining]
		}
		if _, err := out.Write(b); err != nil {
			return err
		}
		remaining -= len(b)
		lba += n
	}
	return nil
}

// detectPSX recognises a PlayStation disc image (for Bleem) and returns its serial, like SLUS00710.
func detectPSX(folder string) (string, bool) {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if isJunk(e.Name()) { // macOS "._" files and the like are not discs
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".cdi" && ext != ".iso" && ext != ".bin" && ext != ".img" && ext != ".mdf" {
			continue
		}
		f, err := os.Open(filepath.Join(folder, e.Name()))
		if err != nil {
			continue
		}
		buf := make([]byte, 4<<20)
		n, _ := io.ReadFull(f, buf)
		f.Close()
		buf = buf[:n]
		if !bytes.Contains(buf, []byte("CD001\x01\x00PLAYSTATION")) {
			continue
		}
		serial := "PSX"
		if i := bytes.Index(buf, []byte("cdrom:\\")); i >= 0 {
			rest := buf[i+7:]
			if j := bytes.IndexAny(rest, ";\r\n "); j > 0 && j < 20 {
				id := strings.ToUpper(string(rest[:j]))
				id = strings.TrimPrefix(id, "\\")
				id = strings.NewReplacer("_", "", ".", "", "\\", "").Replace(id)
				if len(id) > 11 {
					id = id[:11]
				}
				serial = id
			}
		}
		return serial, true
	}
	return "", false
}
