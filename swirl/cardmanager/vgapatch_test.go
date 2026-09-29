package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"hash/adler32"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// vcdVarint writes an RFC 3284 base 128 integer.
func vcdVarint(v int) []byte {
	var out []byte
	for {
		out = append([]byte{byte(v & 0x7f)}, out...)
		v >>= 7
		if v == 0 {
			break
		}
	}
	for i := 0; i < len(out)-1; i++ {
		out[i] |= 0x80
	}
	return out
}

// makeXdelta builds a one window VCDIFF that turns src into dst by copying up to `at`, adding len(repl) bytes
// and copying the rest, the way xdelta3 encodes a small in place change (with its Adler32 window checksum).
func makeXdelta(src []byte, at int, repl []byte) []byte {
	dst := append(append(append([]byte(nil), src[:at]...), repl...), src[at+len(repl):]...)
	var data, inst, addr []byte
	data = append(data, repl...)
	// COPY size (index 19 = mode 0, size follows), ADD size follows (index 1), COPY again
	inst = append(inst, 19)
	inst = append(inst, vcdVarint(at)...)
	addr = append(addr, vcdVarint(0)...)
	inst = append(inst, 1)
	inst = append(inst, vcdVarint(len(repl))...)
	inst = append(inst, 19)
	inst = append(inst, vcdVarint(len(src)-at-len(repl))...)
	addr = append(addr, vcdVarint(at+len(repl))...)
	var w []byte
	w = append(w, 0x05) // VCD_SOURCE | VCD_ADLER32
	w = append(w, vcdVarint(len(src))...)
	w = append(w, vcdVarint(0)...)
	var body []byte
	body = append(body, vcdVarint(len(dst))...)
	body = append(body, 0)
	body = append(body, vcdVarint(len(data))...)
	body = append(body, vcdVarint(len(inst))...)
	body = append(body, vcdVarint(len(addr))...)
	sum := make([]byte, 4)
	binary.BigEndian.PutUint32(sum, adler32.Checksum(dst))
	body = append(body, sum...)
	body = append(body, data...)
	body = append(body, inst...)
	body = append(body, addr...)
	w = append(w, vcdVarint(len(body))...)
	w = append(w, body...)
	out := []byte{0xD6, 0xC3, 0xC4, 0x00, 0x01, 0x02} // header, secondary compressor declared (unused)
	return append(out, w...)
}

func writeDCP(t *testing.T, p string, ip []byte, files map[string][]byte) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if ip != nil {
		w, _ := zw.Create("bootsector/IP.BIN")
		w.Write(ip)
	}
	for n, b := range files {
		w, _ := zw.Create(n)
		w.Write(b)
	}
	zw.Close()
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rawify turns a 2048 byte track into a raw 2352 byte mode 1 track with sync, header, EDC and ECC.
func rawify(t *testing.T, iso string, lba int) string {
	b, err := os.ReadFile(iso)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 0, len(b)/2048*2352)
	for i := 0; (i+1)*2048 <= len(b); i++ {
		s := make([]byte, 2352)
		s[0] = 0
		for j := 1; j < 11; j++ {
			s[j] = 0xFF
		}
		m := lba + i + 150
		s[12] = byte(m / 75 / 60)
		s[13] = byte((m / 75) % 60)
		s[14] = byte(m % 75)
		s[15] = 1
		copy(s[16:], b[i*2048:(i+1)*2048])
		sectorFixMode1(s)
		out = append(out, s...)
	}
	raw := strings.TrimSuffix(iso, ".iso") + ".bin"
	if err := os.WriteFile(raw, out, 0o644); err != nil {
		t.Fatal(err)
	}
	os.Remove(iso)
	return filepath.Base(raw)
}

// testGameFolder builds a game folder from the test menu disc, as 2048 tracks or raw 2352 tracks.
func testGameFolder(t *testing.T, root string, raw bool) string {
	folder := filepath.Join(root, "02")
	w := t.TempDir()
	data, low := filepath.Join(w, "data"), filepath.Join(w, "low")
	os.MkdirAll(data, 0o755)
	os.MkdirAll(low, 0o755)
	bin := make([]byte, 70000)
	for i := range bin {
		bin[i] = byte(i * 7)
	}
	os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), bin, 0o644)
	os.WriteFile(filepath.Join(data, "OTHER.BIN"), []byte(strings.Repeat("x", 5000)), 0o644)
	os.WriteFile(filepath.Join(low, "README.TXT"), []byte("low density"), 0o644)
	os.MkdirAll(folder, 0o755)
	ip := ipSector("TEST GAME", "T-0001N")
	ip[0x38+5] = '0' // no VGA
	if err := buildMenuDisc(data, low, folder, ip); err != nil {
		t.Fatal(err)
	}
	if raw {
		tracks, err := parseGDI(filepath.Join(folder, "disc.gdi"))
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		lines = append(lines, "5")
		for _, tr := range tracks {
			name := filepath.Base(tr.File)
			ss := tr.SectorSize
			if tr.Type == 4 {
				name = rawify(t, tr.File, tr.LBA)
				ss = 2352
			}
			lines = append(lines, strings.Join([]string{itoa(tr.Num), itoa(tr.LBA), itoa(tr.Type), itoa(ss), name, "0"}, " "))
		}
		if err := os.WriteFile(filepath.Join(folder, "disc.gdi"), []byte(strings.Join(lines, "\r\n")+"\r\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return folder
}

func itoa(i int) string { return strconv.Itoa(i) }

func readDiscFileBytes(t *testing.T, folder, name string) []byte {
	d, err := openGDI(filepath.Join(folder, "disc.gdi"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	files, err := listISO(d)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == name {
			b, err := d.readSectors(f.LBA, (f.Size+2047)/2048)
			if err != nil {
				t.Fatal(err)
			}
			return b[:f.Size]
		}
	}
	t.Fatalf("%s not on the disc", name)
	return nil
}

func TestDCPPatchInPlace(t *testing.T) {
	for _, raw := range []bool{false, true} {
		root := t.TempDir()
		folder := testGameFolder(t, root, raw)
		orig := readDiscFileBytes(t, folder, "1ST_READ.BIN")
		at := 12345
		repl := []byte{0x09, 0x00}
		d, _ := openGDI(filepath.Join(folder, "disc.gdi"))
		ipOrig, _ := d.readSectors(d.highDensityStart(), 16)
		d.Close()
		ip := append([]byte(nil), ipOrig...)
		ip[0x3D] = '1' // VGA flag in the peripherals field
		dcp := filepath.Join(root, "Game (VGA Patch).dcp")
		writeDCP(t, dcp, ip, map[string][]byte{"1ST_READ.BIN.xdelta": makeXdelta(orig, at, repl)})

		rep, err := applyDCP(root, folder, dcp, true)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Sectors != 2 || !rep.DryRun {
			t.Fatalf("dry run: %+v", rep)
		}
		if b := readDiscFileBytes(t, folder, "1ST_READ.BIN"); !bytes.Equal(b, orig) {
			t.Fatal("dry run wrote to the disc")
		}

		rep, err = applyDCP(root, folder, dcp, false)
		if err != nil {
			t.Fatal(err)
		}
		if rep.Sectors != 2 || rep.UndoFile == "" {
			t.Fatalf("apply: %+v", rep)
		}
		got := readDiscFileBytes(t, folder, "1ST_READ.BIN")
		want := append(append(append([]byte(nil), orig[:at]...), repl...), orig[at+2:]...)
		if !bytes.Equal(got, want) {
			t.Fatal("1ST_READ.BIN not patched as expected")
		}
		info, _, err := readImageIP(folder)
		if err != nil || !info.VGA {
			t.Fatalf("IP.BIN VGA flag not set after the patch: %v %+v", err, info)
		}
		if raw {
			// every raw sector still carries valid check bytes
			tracks, _ := parseGDI(filepath.Join(folder, "disc.gdi"))
			for _, tr := range tracks {
				if tr.SectorSize != 2352 {
					continue
				}
				b, _ := os.ReadFile(tr.File)
				for i := 0; i+2352 <= len(b); i += 2352 {
					s := append([]byte(nil), b[i:i+2352]...)
					sectorFixMode1(s)
					if !bytes.Equal(s, b[i:i+2352]) {
						t.Fatalf("track %d sector %d has bad EDC/ECC after the patch", tr.Num, i/2352)
					}
				}
			}
		}
		notes, _ := os.ReadFile(filepath.Join(folder, "patches.txt"))
		if !strings.Contains(string(notes), "Game (VGA Patch)") {
			t.Fatal("patches.txt not written")
		}

		// applying again changes nothing
		rep, err = applyDCP(root, folder, dcp, true)
		if err != nil || rep.Sectors != 0 {
			t.Fatalf("second apply: %v %+v", err, rep)
		}

		// undo puts every byte back
		if _, err := undoDCP(root, folder, rep2Undo(t, root, folder)); err != nil {
			t.Fatal(err)
		}
		if b := readDiscFileBytes(t, folder, "1ST_READ.BIN"); !bytes.Equal(b, orig) {
			t.Fatal("undo did not restore 1ST_READ.BIN")
		}
		d, _ = openGDI(filepath.Join(folder, "disc.gdi"))
		ipNow, _ := d.readSectors(d.highDensityStart(), 16)
		d.Close()
		if !bytes.Equal(ipNow, ipOrig) {
			t.Fatal("undo did not restore IP.BIN")
		}
	}
}

func rep2Undo(t *testing.T, root, folder string) string {
	m, _ := filepath.Glob(filepath.Join(patchDir(root, folder), "*.undo"))
	if len(m) != 1 {
		t.Fatalf("undo files: %v", m)
	}
	return m[0]
}

func TestDCPRefusals(t *testing.T) {
	root := t.TempDir()
	folder := testGameFolder(t, root, false)
	orig := readDiscFileBytes(t, folder, "1ST_READ.BIN")

	// a patch made for another version of the file: the window checksum does not match
	other := append([]byte(nil), orig...)
	other[100] ^= 0xFF
	dcp := filepath.Join(root, "wrong.dcp")
	writeDCP(t, dcp, nil, map[string][]byte{"1ST_READ.BIN.xdelta": makeXdelta(other, 500, []byte{1, 2})})
	if _, err := applyDCP(root, folder, dcp, false); err == nil || !strings.Contains(err.Error(), "not the one this patch was made for") {
		t.Fatalf("wrong version: %v", err)
	}
	if b := readDiscFileBytes(t, folder, "1ST_READ.BIN"); !bytes.Equal(b, orig) {
		t.Fatal("a refused patch changed the disc")
	}

	// a file that is not on the disc
	dcp = filepath.Join(root, "missing.dcp")
	writeDCP(t, dcp, nil, map[string][]byte{"NOPE.BIN.xdelta": makeXdelta(orig, 500, []byte{1, 2})})
	if _, err := applyDCP(root, folder, dcp, false); err == nil || !strings.Contains(err.Error(), "not on this disc") {
		t.Fatalf("missing file: %v", err)
	}

	// a whole file of another size needs a rebuild
	dcp = filepath.Join(root, "size.dcp")
	writeDCP(t, dcp, nil, map[string][]byte{"1ST_READ.BIN": append(orig, 1, 2, 3)})
	if _, err := applyDCP(root, folder, dcp, false); err == nil || !strings.Contains(err.Error(), "rebuilt image") {
		t.Fatalf("size change: %v", err)
	}

	// not a zip
	os.WriteFile(filepath.Join(root, "junk.dcp"), []byte("hello"), 0o644)
	if _, err := applyDCP(root, folder, filepath.Join(root, "junk.dcp"), false); err == nil {
		t.Fatal("junk accepted")
	}

	// a CDI folder
	cdi := filepath.Join(root, "03")
	os.MkdirAll(cdi, 0o755)
	os.WriteFile(filepath.Join(cdi, "disc.cdi"), make([]byte, 4096), 0o644)
	if _, err := applyDCP(root, cdi, dcp, false); err == nil || !strings.Contains(err.Error(), "GDI") {
		t.Fatalf("cdi: %v", err)
	}
}

func TestVGAPatchCatalog(t *testing.T) {
	cat := []vgaPatchEntry{{Product: "T-9702N", Version: "V1.020", Name: "Hydro Thunder"}}
	if findVGAPatch(cat, "t-9702n", "v1.020") == nil {
		t.Fatal("case insensitive match failed")
	}
	if findVGAPatch(cat, "T-9702N", "V1.000") != nil {
		t.Fatal("another version matched")
	}
	if findVGAPatch(cat, "T-9703N", "V1.020") != nil {
		t.Fatal("another product matched")
	}
}

func TestVGACatalogEmbedded(t *testing.T) {
	e := vgaPatchFor("T-9702N", "V1.020")
	if e == nil {
		t.Fatal("Hydro Thunder is not in the catalog")
	}
	p, err := writeCatalogPatch(e)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(p))
	d, err := readDCP(p)
	if err != nil {
		t.Fatal(err)
	}
	if d.IPBIN == nil || len(d.Files) != 1 || d.Files[0].Path != "1ST_READ.BIN" {
		t.Fatalf("unexpected patch contents: %+v", d.Files)
	}
	// the delta is a two byte change in a 1 888 290 byte file: a wrong file is refused by its checksum
	if _, err := vcdiffApply(d.Files[0].Delta, make([]byte, 1888290)); err == nil {
		t.Fatal("a zero file was accepted")
	}
	if vgaPatchFor("T-9702N", "V1.000") != nil {
		t.Fatal("another version matched")
	}
}

func TestVGAFixAllReportsEachGame(t *testing.T) {
	root := t.TempDir()
	testGameFolder(t, root, false) // T-0001N, not in the catalog
	rows, err := vgaFixAll(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !strings.Contains(rows[0], "no game on this card") {
		t.Fatalf("rows: %v", rows)
	}
}

// A card whose game has a patch in the owner's own patch folder: the scan reports it, the card wide fix
// patches it, a second scan shows it patched, and the undo puts it back.
func TestVGAScanFixUndoFlow(t *testing.T) {
	root := t.TempDir()
	makeMenuIn01(t, root, "SWIRL")
	folder := testGameFolder(t, root, true) // T-0001N V1.000, no VGA flag, raw tracks
	orig := readDiscFileBytes(t, folder, "1ST_READ.BIN")
	pdir := t.TempDir()
	t.Setenv("SWIRL_PATCH_DIR", pdir)
	d, _ := openGDI(filepath.Join(folder, "disc.gdi"))
	ip, _ := d.readSectors(d.highDensityStart(), 16)
	d.Close()
	ip = append([]byte(nil), ip...)
	ip[0x3D] = '1'
	writeDCP(t, filepath.Join(pdir, "test.dcp"), ip, map[string][]byte{"1ST_READ.BIN.xdelta": makeXdelta(orig, 777, []byte{9, 0})})
	os.WriteFile(filepath.Join(pdir, "catalog.json"), []byte(`[{"product":"T0001N","version":"V1.000","name":"Test","author":"Tester","file":"test.dcp"}]`), 0o644)

	c, err := ScanCard(root)
	if err != nil {
		t.Fatal(err)
	}
	g := c.Games[0]
	if g.VGAState != "patch" || g.VGABy != "Tester" || g.VGA {
		t.Fatalf("before: %+v", g)
	}
	res, err := vgaFixCard(root)
	if err != nil || len(res.Patched) != 1 || len(res.Failed) != 0 {
		t.Fatalf("fix: %+v %v", res, err)
	}
	c, _ = ScanCard(root)
	if g := c.Games[0]; g.VGAState != "patched" || !g.VGA {
		t.Fatalf("after: %+v", g)
	}
	res, _ = vgaFixCard(root)
	if len(res.Already) != 1 || len(res.Patched) != 0 {
		t.Fatalf("second fix: %+v", res)
	}
	undo := vgaUndoFile(root, folder)
	if undo == "" {
		t.Fatal("no undo file")
	}
	if _, err := undoDCP(root, folder, undo); err != nil {
		t.Fatal(err)
	}
	c, _ = ScanCard(root)
	if g := c.Games[0]; g.VGAState != "skipped" || g.VGA {
		t.Fatalf("after undo: %+v", g)
	}
	if b := readDiscFileBytes(t, folder, "1ST_READ.BIN"); !bytes.Equal(b, orig) {
		t.Fatal("undo did not restore the file")
	}
	// a patch removed by hand is not put back by the automatic pass
	res, _ = vgaFixCard(root)
	if len(res.Patched) != 0 || len(res.Already) != 0 {
		t.Fatalf("fix after a removal: %+v", res)
	}
	// applying it on purpose works again and ends the skip
	if _, err := applyCatalogVGAPatch(root, folder, false); err != nil {
		t.Fatal(err)
	}
	c, _ = ScanCard(root)
	if g := c.Games[0]; g.VGAState != "patched" {
		t.Fatalf("after re apply: %+v", g)
	}
}
