package main

// Minimal ISO9660 writer. Files are placed at absolute LBAs starting at `base`, which lets the same
// code build the low density track (base 0) and the GD-ROM high density track (base 45000).

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type isoNode struct {
	id       string // on-disc identifier (upper case, ";1" on files)
	src      string
	dir      bool
	size     int64
	children []*isoNode
	parent   *isoNode
	lba      int
	dirBytes int
	num      int
}

func isoName(n string) string {
	n = strings.ToUpper(n)
	var b strings.Builder
	for _, r := range n {
		if (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func loadTree(dir string, parent *isoNode) (*isoNode, error) {
	node := &isoNode{src: dir, dir: true, parent: parent}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		p := filepath.Join(dir, e.Name())
		if e.IsDir() {
			c, err := loadTree(p, node)
			if err != nil {
				return nil, err
			}
			c.id = isoName(e.Name())
			node.children = append(node.children, c)
		} else {
			st, err := os.Stat(p)
			if err != nil {
				return nil, err
			}
			id := isoName(e.Name())
			if !strings.Contains(id, ".") {
				id += "."
			}
			node.children = append(node.children, &isoNode{id: id + ";1", src: p, size: st.Size(), parent: node})
		}
	}
	sort.Slice(node.children, func(i, j int) bool { return node.children[i].id < node.children[j].id })
	return node, nil
}

func recLen(id string) int {
	l := 33 + len(id)
	if l%2 == 1 {
		l++
	}
	return l
}

func bothEndian32(b []byte, v uint32) {
	binary.LittleEndian.PutUint32(b, v)
	binary.BigEndian.PutUint32(b[4:], v)
}

func bothEndian16(b []byte, v uint16) {
	binary.LittleEndian.PutUint16(b, v)
	binary.BigEndian.PutUint16(b[2:], v)
}

func isoDate7(t time.Time) []byte {
	return []byte{byte(t.Year() - 1900), byte(t.Month()), byte(t.Day()), byte(t.Hour()), byte(t.Minute()), byte(t.Second()), 0}
}

func dirRecord(id []byte, lba int, size int64, dir bool, t time.Time) []byte {
	l := 33 + len(id)
	if l%2 == 1 {
		l++
	}
	r := make([]byte, l)
	r[0] = byte(l)
	bothEndian32(r[2:], uint32(lba))
	bothEndian32(r[10:], uint32(size))
	copy(r[18:25], isoDate7(t))
	if dir {
		r[25] = 2
	}
	bothEndian16(r[28:], 1)
	r[32] = byte(len(id))
	copy(r[33:], id)
	return r
}

func padStr(b []byte, s string) {
	for i := range b {
		b[i] = ' '
	}
	copy(b, s)
}

// buildISO writes srcDir as an ISO9660 image to out. ip (32 KB) fills the system area when given.
func buildISO(srcDir, out string, base int, volID string, ip []byte) error {
	_, err := buildISOSplit(srcDir, out, "", base, 0, volID, ip, "")
	return err
}

// buildISOSplit writes the volume descriptors and directories to metaOut (starting at LBA base). When
// dataOut is set, file contents go to dataOut and are placed so the last file ends right before endLBA,
// which is the GD-ROM layout GDMENUCardManager uses (data at the outer edge of the disc). It returns
// the LBA where dataOut starts.
func buildISOSplit(srcDir, metaOut, dataOut string, base, endLBA int, volID string, ip []byte, bootName string) (int, error) {
	root, err := loadTree(srcDir, nil)
	if err != nil {
		return 0, err
	}
	root.id = "\x00"
	now := time.Now()

	// directories in breadth first order
	dirs := []*isoNode{root}
	for i := 0; i < len(dirs); i++ {
		for _, c := range dirs[i].children {
			if c.dir {
				dirs = append(dirs, c)
			}
		}
	}
	for i, d := range dirs {
		d.num = i + 1
		// '.' and '..' plus children, never splitting a record across sectors
		used, total := 0, 0
		add := func(n int) {
			if used+n > sectorSize {
				total += sectorSize
				used = 0
			}
			used += n
		}
		add(34)
		add(34)
		for _, c := range d.children {
			add(recLen(c.id))
		}
		total += sectorSize
		d.dirBytes = total
	}
	// path table
	var pt []byte
	for _, d := range dirs {
		id := d.id
		if d == root {
			id = "\x00"
		}
		e := make([]byte, 8+len(id)+len(id)%2)
		e[0] = byte(len(id))
		parent := 1
		if d.parent != nil {
			parent = d.parent.num
		}
		binary.LittleEndian.PutUint16(e[6:], uint16(parent))
		copy(e[8:], id)
		pt = append(pt, e...)
	}
	ptSectors := (len(pt) + sectorSize - 1) / sectorSize
	lba := 18
	lPath := lba
	lba += ptSectors
	mPath := lba
	lba += ptSectors
	for _, d := range dirs {
		d.lba = base + lba
		lba += d.dirBytes / sectorSize
	}
	var files []*isoNode
	var collect func(n *isoNode)
	collect = func(n *isoNode) {
		for _, c := range n.children {
			if c.dir {
				collect(c)
			} else {
				files = append(files, c)
			}
		}
	}
	collect(root)
	metaSectors := lba
	fileSectors := 0
	for _, f := range files {
		fileSectors += int((f.size + sectorSize - 1) / sectorSize)
	}
	dataStart := base + lba
	if dataOut != "" {
		// GD-ROM layout (same as GDMENUCardManager / GDI Builder): the boot file named in IP.BIN is the
		// last file and ends 150 sectors before the end of the disc; every other file sits right before it.
		var boot *isoNode
		for _, f := range files {
			if f.parent == root && strings.EqualFold(strings.TrimSuffix(f.id, ";1"), bootName) {
				boot = f
			}
		}
		ordered := make([]*isoNode, 0, len(files))
		for _, f := range files {
			if f != boot {
				ordered = append(ordered, f)
			}
		}
		if boot != nil {
			ordered = append(ordered, boot)
		}
		files = ordered
		dataStart = endLBA - 150 - fileSectors
		if dataStart-151 <= base+metaSectors+150 {
			return 0, errors.New("menu data is too large for a GD-ROM")
		}
		fileSectors = endLBA - dataStart // track runs to the end of the disc, zero padded
	}
	next := dataStart
	for _, f := range files {
		f.lba = next
		next += int((f.size + sectorSize - 1) / sectorSize)
	}
	totalSectors := metaSectors + fileSectors
	volSize := base + totalSectors
	if dataOut != "" {
		volSize = endLBA - base
	}

	w, err := os.Create(metaOut)
	if err != nil {
		return 0, err
	}
	defer w.Close()
	dw := w
	if dataOut != "" {
		if dw, err = os.Create(dataOut); err != nil {
			return 0, err
		}
		defer dw.Close()
		if err := w.Truncate(int64(metaSectors) * sectorSize); err != nil {
			return 0, err
		}
		if err := dw.Truncate(int64(fileSectors) * sectorSize); err != nil {
			return 0, err
		}
	} else if err := w.Truncate(int64(totalSectors) * sectorSize); err != nil {
		return 0, err
	}
	if len(ip) > 0 {
		if len(ip) > 16*sectorSize {
			ip = ip[:16*sectorSize]
		}
		w.WriteAt(ip, 0)
	}

	// path tables (L little endian extents, M big endian)
	lpt := make([]byte, len(pt))
	mpt := make([]byte, len(pt))
	copy(lpt, pt)
	copy(mpt, pt)
	pos := 0
	for _, d := range dirs {
		idl := int(pt[pos])
		binary.LittleEndian.PutUint32(lpt[pos+2:], uint32(d.lba))
		binary.BigEndian.PutUint32(mpt[pos+2:], uint32(d.lba))
		binary.BigEndian.PutUint16(mpt[pos+6:], binary.LittleEndian.Uint16(pt[pos+6:]))
		pos += 8 + idl + idl%2
	}
	w.WriteAt(lpt, int64(lPath)*sectorSize)
	w.WriteAt(mpt, int64(mPath)*sectorSize)

	// directory extents
	for _, d := range dirs {
		buf := make([]byte, d.dirBytes)
		used, off := 0, 0
		put := func(r []byte) {
			if used+len(r) > sectorSize {
				off += sectorSize
				used = 0
			}
			copy(buf[off+used:], r)
			used += len(r)
		}
		parent := d
		if d.parent != nil {
			parent = d.parent
		}
		put(dirRecord([]byte{0}, d.lba, int64(d.dirBytes), true, now))
		put(dirRecord([]byte{1}, parent.lba, int64(parent.dirBytes), true, now))
		for _, c := range d.children {
			if c.dir {
				put(dirRecord([]byte(c.id), c.lba, int64(c.dirBytes), true, now))
			} else {
				put(dirRecord([]byte(c.id), c.lba, c.size, false, now))
			}
		}
		w.WriteAt(buf, int64(d.lba-base)*sectorSize)
	}

	// files
	for _, f := range files {
		in, err := os.Open(f.src)
		if err != nil {
			return 0, err
		}
		var sw *io.OffsetWriter
		if dataOut != "" {
			sw = io.NewOffsetWriter(dw, int64(f.lba-dataStart)*sectorSize)
		} else {
			sw = io.NewOffsetWriter(w, int64(f.lba-base)*sectorSize)
		}
		_, err = io.Copy(sw, in)
		in.Close()
		if err != nil {
			return 0, err
		}
	}

	// primary volume descriptor
	pvd := make([]byte, sectorSize)
	pvd[0] = 1
	copy(pvd[1:], "CD001")
	pvd[6] = 1
	padStr(pvd[8:40], "")
	padStr(pvd[40:72], volID)
	bothEndian32(pvd[80:], uint32(volSize))
	bothEndian16(pvd[120:], 1)
	bothEndian16(pvd[124:], 1)
	bothEndian16(pvd[128:], sectorSize)
	bothEndian32(pvd[132:], uint32(len(pt)))
	binary.LittleEndian.PutUint32(pvd[140:], uint32(base+lPath))
	binary.BigEndian.PutUint32(pvd[148:], uint32(base+mPath))
	copy(pvd[156:], dirRecord([]byte{0}, root.lba, int64(root.dirBytes), true, now))
	padStr(pvd[190:318], volID)
	padStr(pvd[318:446], "SWIRL")
	padStr(pvd[446:574], "SWIRL CARD MANAGER")
	padStr(pvd[574:702], "SWIRL")
	padStr(pvd[702:813], "")
	stamp := now.Format("20060102150405") + "00"
	for _, o := range []int{813, 830} {
		copy(pvd[o:], stamp)
	}
	for _, o := range []int{847, 864} {
		copy(pvd[o:], "0000000000000000")
	}
	pvd[881] = 1
	w.WriteAt(pvd, 16*sectorSize)
	term := make([]byte, sectorSize)
	term[0] = 255
	copy(term[1:], "CD001")
	term[6] = 1
	w.WriteAt(term, 17*sectorSize)
	return dataStart, nil
}
