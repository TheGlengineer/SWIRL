package main

// Per game edits kept on the SD card in SWIRL/ (GDEMU ignores folders that are not numbers).
//   SWIRL/games.json            text and metadata edits, keyed by folder
//   SWIRL/art/<folder>_box.png  cover art (becomes BOX.DAT 256x256 and ICON.DAT 128x128)
//   SWIRL/art/<folder>_vmu.bin  48x32 VMU screen image (becomes VMU.DAT)
// Names and serials are also written to name.txt / serial.txt in the game folder, the same files
// GDMENUCardManager uses, so both tools agree.

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

const editsDir = "SWIRL"

type GameEdit struct {
	Product string `json:"product"` // serial the edit was made against
	Region  string `json:"region,omitempty"`
	VGA     *bool  `json:"vga,omitempty"`
	Date    string `json:"date,omitempty"`
	Meta    *Meta  `json:"meta,omitempty"`
	// UserName is set when the name was typed in the Edit window, so Tidy names leaves it alone
	UserName bool `json:"userName,omitempty"`
}

type editStore struct {
	root  string
	Games map[string]*GameEdit `json:"games"`
}

func loadEdits(root string) *editStore {
	s := &editStore{root: root, Games: map[string]*GameEdit{}}
	if b, err := os.ReadFile(filepath.Join(root, editsDir, "games.json")); err == nil {
		json.Unmarshal(b, s)
		if s.Games == nil {
			s.Games = map[string]*GameEdit{}
		}
	}
	return s
}

func (s *editStore) save() error {
	if err := os.MkdirAll(filepath.Join(s.root, editsDir, "art"), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(filepath.Join(s.root, editsDir, "games.json"), b, 0o644)
}

func artPath(root, folder, kind string) string {
	ext := ".png"
	if kind == "vmu" {
		ext = ".bin"
	}
	return filepath.Join(root, editsDir, "art", folder+"_"+kind+ext)
}

func decodeDataURL(s string) (image.Image, error) {
	if i := strings.Index(s, ","); i >= 0 {
		s = s[i+1:]
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(b))
	return img, err
}

// ---------- reading the current menu disc ----------

type menuReader struct {
	d     *gdiDisc
	files map[string]isoFile
}

func openMenuDisc(root string) (*menuReader, error) {
	gdi := findGDI(filepath.Join(root, "01"))
	if gdi == "" {
		return nil, os.ErrNotExist
	}
	d, err := openGDI(gdi)
	if err != nil {
		return nil, err
	}
	files, err := listISO(d)
	if err != nil {
		d.Close()
		return nil, err
	}
	m := &menuReader{d: d, files: map[string]isoFile{}}
	for _, f := range files {
		m.files[strings.ToUpper(f.Path)] = f
	}
	return m, nil
}

func (m *menuReader) Close() { m.d.Close() }

// datChunk reads one entry of a DAT file on the menu disc without extracting the whole file.
func (m *menuReader) datChunk(dat, id string) ([]byte, error) {
	f, ok := m.files[dat]
	if !ok {
		return nil, os.ErrNotExist
	}
	hdr, err := m.d.readSectors(f.LBA, 1)
	if err != nil || string(hdr[:3]) != "DAT" {
		return nil, errors.New("bad DAT")
	}
	cs := int(binary.LittleEndian.Uint32(hdr[4:]))
	n := int(binary.LittleEndian.Uint32(hdr[8:]))
	tbl, err := m.d.readSectors(f.LBA, (16+16*n+sectorSize-1)/sectorSize)
	if err != nil {
		return nil, err
	}
	id = datID(id)
	for i := 0; i < n; i++ {
		rec := tbl[16+16*i : 32+16*i]
		if string(bytes.TrimRight(rec[:12], "\x00")) != id {
			continue
		}
		off := int(binary.LittleEndian.Uint32(rec[12:])) * cs
		first := off / sectorSize
		b, err := m.d.readSectors(f.LBA+first, (off%sectorSize+cs+sectorSize-1)/sectorSize)
		if err != nil {
			return nil, err
		}
		return b[off%sectorSize : off%sectorSize+cs], nil
	}
	return nil, os.ErrNotExist
}
