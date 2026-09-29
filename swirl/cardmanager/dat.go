package main

// openMenu DAT files: "DAT" + version 1, chunk size, chunk count, then a table of 12 byte IDs with chunk
// indexes, then fixed size chunks. BOX.DAT / ICON.DAT chunks hold PVR textures, META.DAT chunks hold db_item.

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
)

type datFile struct {
	ChunkSize int
	Order     []string
	Chunks    map[string][]byte
}

func newDat(chunk int) *datFile {
	return &datFile{ChunkSize: chunk, Chunks: map[string][]byte{}}
}

// checkDatHeader reads the 16 byte header of a DAT file of size bytes and says what is wrong with it.
// The menu's reader (dat_reader.c) accepts the same files, so anything refused here is unreadable on
// the Dreamcast too.
func checkDatHeader(hdr []byte, size int64) (chunk, count int, err error) {
	if len(hdr) < 16 || string(hdr[0:3]) != "DAT" {
		return 0, 0, errors.New("not a DAT file")
	}
	if hdr[3] != 1 {
		return 0, 0, fmt.Errorf("version %d, SWIRL reads version 1", hdr[3])
	}
	chunk = int(binary.LittleEndian.Uint32(hdr[4:]))
	count = int(binary.LittleEndian.Uint32(hdr[8:]))
	switch {
	case chunk <= 0 || chunk > 1<<24:
		return 0, 0, fmt.Errorf("chunk size %d makes no sense", chunk)
	case count < 0 || count > 100000:
		return 0, 0, fmt.Errorf("%d entries makes no sense", count)
	case int64(16+16*count) > size:
		return 0, 0, fmt.Errorf("the table of %d entries runs past the end of the file", count)
	}
	return chunk, count, nil
}

func parseDat(b []byte) (*datFile, error) {
	cs, n, err := checkDatHeader(b, int64(len(b)))
	if err != nil {
		return nil, err
	}
	d := newDat(cs)
	for i := 0; i < n; i++ {
		rec := b[16+16*i : 32+16*i]
		id := string(bytes.TrimRight(rec[:12], "\x00"))
		idx := int(binary.LittleEndian.Uint32(rec[12:]))
		off := idx * cs
		if off < 0 || off+cs > len(b) {
			continue
		}
		if _, dup := d.Chunks[id]; !dup {
			d.Order = append(d.Order, id)
		}
		c := make([]byte, cs)
		copy(c, b[off:off+cs])
		d.Chunks[id] = c
	}
	return d, nil
}

func readDat(path string) (*datFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseDat(b)
}

func (d *datFile) Set(id string, data []byte) {
	id = datID(id)
	if id == "" {
		return
	}
	c := make([]byte, d.ChunkSize)
	copy(c, data)
	if _, ok := d.Chunks[id]; !ok {
		d.Order = append(d.Order, id)
	}
	d.Chunks[id] = c
}

func datID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) > 11 {
		id = id[:11]
	}
	return id
}

func (d *datFile) Write(path string) error {
	ids := append([]string(nil), d.Order...)
	sort.Strings(ids)
	n := len(ids)
	first := (16 + 16*n + d.ChunkSize - 1) / d.ChunkSize
	if first < 1 {
		first = 1
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := make([]byte, first*d.ChunkSize)
	copy(hdr, "DAT\x01")
	binary.LittleEndian.PutUint32(hdr[4:], uint32(d.ChunkSize))
	binary.LittleEndian.PutUint32(hdr[8:], uint32(n))
	for i, id := range ids {
		copy(hdr[16+16*i:16+16*i+12], id)
		binary.LittleEndian.PutUint32(hdr[16+16*i+12:], uint32(first+i))
	}
	if _, err := f.Write(hdr); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := f.Write(d.Chunks[id]); err != nil {
			return err
		}
	}
	return nil
}

// ---------- META.DAT records (openMenu db_item) ----------

const metaSize = 384

type Meta struct {
	Players     int    `json:"players"`
	VMUBlocks   int    `json:"vmuBlocks"`
	Network     int    `json:"network"` // 0 none, 1 modem, 2 BBA, 3 both
	Genre       uint16 `json:"genre"`
	Accessories uint16 `json:"accessories"`
	Description string `json:"description"`
}

func decodeMeta(b []byte) Meta {
	if len(b) < metaSize {
		return Meta{}
	}
	return Meta{
		Players:     int(b[0]),
		VMUBlocks:   int(b[1]),
		Network:     int(b[3]),
		Genre:       binary.LittleEndian.Uint16(b[4:]),
		Accessories: binary.LittleEndian.Uint16(b[6:]),
		Description: string(bytes.TrimRight(b[8:metaSize], "\x00")),
	}
}

func encodeMeta(m Meta) []byte {
	b := make([]byte, metaSize)
	b[0] = byte(clamp(m.Players, 0, 255))
	b[1] = byte(clamp(m.VMUBlocks, 0, 255))
	b[3] = byte(clamp(m.Network, 0, 3))
	binary.LittleEndian.PutUint16(b[4:], m.Genre)
	binary.LittleEndian.PutUint16(b[6:], m.Accessories)
	desc := asciiOnly(m.Description)
	if len(desc) > 375 {
		desc = desc[:375]
	}
	copy(b[8:], desc)
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func asciiOnly(s string) string {
	repl := strings.NewReplacer("‘", "'", "’", "'", "“", "\"", "”", "\"", "–", "-", "—", "-", "…", "...", "\r", "", "\n", " ")
	s = repl.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r < 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}
