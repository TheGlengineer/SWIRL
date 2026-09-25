package main

// openMenu DAT files: "DAT" + version 1, chunk size, chunk count, then a table of 12 byte IDs with chunk
// indexes, then fixed size chunks. BOX.DAT / ICON.DAT chunks hold PVR textures, META.DAT chunks hold db_item.

import (
	"bytes"
	"encoding/binary"
	"errors"
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

func parseDat(b []byte) (*datFile, error) {
	if len(b) < 16 || string(b[0:3]) != "DAT" || b[3] != 1 {
		return nil, errors.New("not a DAT file")
	}
	cs := int(binary.LittleEndian.Uint32(b[4:]))
	n := int(binary.LittleEndian.Uint32(b[8:]))
	if cs <= 0 || n < 0 || 16+16*n > len(b) {
		return nil, errors.New("bad DAT header")
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
