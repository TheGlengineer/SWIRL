package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// menuWithDat puts an openMenu menu in 01 whose BOX.DAT holds ids and has version in its header byte.
func menuWithDat(t *testing.T, root string, ids []string, version byte) []byte {
	t.Helper()
	d := newDat(131104)
	for _, id := range ids {
		d.Set(id, []byte("art for "+id))
	}
	p := filepath.Join(t.TempDir(), "BOX.DAT")
	if err := d.Write(p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	b[3] = version
	makeMenuIn01With(t, root, "openMenu", map[string][]byte{"BOX.DAT": b})
	return b
}

func menuFile(t *testing.T, root, name string) []byte {
	t.Helper()
	m, err := openMenuDisc(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	f, ok := m.files[name]
	if !ok {
		return nil
	}
	b, err := m.d.readSectors(f.LBA, (f.Size+sectorSize-1)/sectorSize)
	if err != nil {
		t.Fatal(err)
	}
	return b[:f.Size]
}

// CM-11 and CM-24: a DAT the menu cannot read is reported by the scan and carried over untouched by every
// rebuild, with edits and the online database in play.
func TestUnreadableDatKept(t *testing.T) {
	db := t.TempDir()
	for _, d := range []struct {
		n string
		c int
	}{{"BOX.DAT", 131104}, {"ICON.DAT", 32800}, {"META.DAT", metaSize}} {
		x := newDat(d.c)
		x.Set("T00004N", []byte("online art"))
		x.Write(filepath.Join(db, d.n))
	}
	b, _ := json.Marshal(dbInfo{Downloaded: time.Now()})
	os.WriteFile(filepath.Join(db, "info.json"), b, 0o644)
	for _, tc := range []struct {
		name    string
		edit    bool
		online  bool
		version byte
	}{{"v2 no edits", false, false, 2}, {"v2 with a box edit", true, false, 2}, {"v2 with the online database", false, true, 2}, {"v2 with both", true, true, 2}, {"v1 control", true, true, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.online {
				dbDirOverride = db
				defer func() { dbDirOverride = "" }()
			}
			root := txnCard(t, "", 3)
			orig := menuWithDat(t, root, []string{"T00002N", "T00003N"}, tc.version)
			if tc.edit {
				os.WriteFile(artPath(root, "04", "box"), pngBytes(quadImage(64)), 0o644)
			}
			c, err := ScanCard(root)
			if err != nil {
				t.Fatal(err)
			}
			if tc.version != 1 {
				if len(c.DatIssues) != 1 || !strings.Contains(c.DatIssues[0], "BOX.DAT on the menu disc cannot be read (version 2, SWIRL reads version 1)") {
					t.Fatalf("dat issues %q", c.DatIssues)
				}
				if !strings.Contains(strings.Join(c.Warnings, "\n"), "BOX.DAT") {
					t.Fatalf("warnings %q", c.Warnings)
				}
				if findGame(c, "02").HasArt {
					t.Fatal("art reported from a DAT the menu cannot read")
				}
				h, _ := CheckCard(root)
				found := false
				for _, it := range h.Items {
					if it.Folder == "01" && strings.Contains(it.Message, "BOX.DAT") {
						found = true
					}
				}
				if !found {
					t.Fatalf("health items %+v", h.Items)
				}
			} else if len(c.DatIssues) != 0 || !findGame(c, "02").HasArt {
				t.Fatalf("v1: issues %q, hasArt %v", c.DatIssues, findGame(c, "02").HasArt)
			}
			var lines []string
			logf := func(f string, a ...any) { lines = append(lines, fmt.Sprintf(f, a...)) }
			for i := 0; i < 2; i++ {
				if err := InstallSwirl(root, "", logf); err != nil {
					t.Fatal(err)
				}
			}
			got := menuFile(t, root, "BOX.DAT")
			if tc.version != 1 {
				if string(got) != string(orig) {
					t.Fatalf("BOX.DAT was changed: %d bytes, was %d", len(got), len(orig))
				}
				if (tc.edit || tc.online) && !strings.Contains(strings.Join(lines, "\n"), "BOX.DAT on the current menu cannot be read") {
					t.Fatalf("log %q", lines)
				}
				if tc.edit {
					icon := menuFile(t, root, "ICON.DAT")
					if d, err := parseDat(icon); err != nil || d.Chunks["T00004N"] == nil {
						t.Fatalf("the edit was not saved to ICON.DAT, which is readable: %v", err)
					}
				}
			} else {
				d, err := parseDat(got)
				if err != nil {
					t.Fatal(err)
				}
				if d.Chunks["T00002N"] == nil || d.Chunks["T00004N"] == nil {
					t.Fatalf("v1 BOX.DAT ids %v", d.Order)
				}
			}
		})
	}
}

func TestCheckDatHeader(t *testing.T) {
	hdr := func(ver byte, chunk, n uint32) []byte {
		b := []byte("DAT\x01\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00")
		b[3] = ver
		b[4], b[5], b[6], b[7] = byte(chunk), byte(chunk>>8), byte(chunk>>16), byte(chunk>>24)
		b[8], b[9], b[10], b[11] = byte(n), byte(n>>8), byte(n>>16), byte(n>>24)
		return b
	}
	for _, tc := range []struct {
		h    []byte
		size int64
		want string
	}{
		{hdr(1, 131104, 2), 393312, ""},
		{hdr(2, 131104, 2), 393312, "version 2"},
		{hdr(1, 131104, 50000), 393312, "runs past the end"},
		{hdr(1, 0, 2), 393312, "chunk size"},
		{[]byte("PVRT"), 100, "not a DAT"},
		{hdr(1, 131104, 0), 16, ""},
	} {
		_, _, err := checkDatHeader(tc.h, tc.size)
		switch {
		case tc.want == "" && err != nil:
			t.Errorf("%q: %v", tc.h[:4], err)
		case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
			t.Errorf("want %q, got %v", tc.want, err)
		}
	}
}
