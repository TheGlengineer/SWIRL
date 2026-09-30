package main

// Applying .dcp patches (Universal Dreamcast Patcher format) to a game folder on the card, in place.
//
// A .dcp is a zip: files of the disc's filesystem, either whole or as xdelta patches named "<file>.xdelta",
// plus bootsector/IP.BIN. Universal Dreamcast Patcher rebuilds the whole image; SWIRL writes only the sectors
// that change, straight into the GDI's track files, with fresh EDC/ECC for raw sectors. Nothing moves on the
// disc, so the GDEMU layout, the CDDA tracks and the folder's other files are untouched. The original bytes of
// every written sector go to SWIRL_BACKUP/patches/<folder>/ first, so a patch can be taken off again.
//
// A patch that would change a file's size, or that is for a CDI, needs a rebuilt image and is refused with a
// message that says so.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type dcpFile struct {
	Path  string // inside the disc, forward slashes, upper case
	Delta []byte // xdelta patch, or
	Whole []byte // the complete file
}

type dcpPatch struct {
	Name   string
	SHA256 string
	IPBIN  []byte
	Files  []dcpFile
}

func readDCP(p string) (*dcpPatch, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return nil, fmt.Errorf("%s is not a .dcp patch (not a zip)", filepath.Base(p))
	}
	sum := sha256.Sum256(raw)
	d := &dcpPatch{Name: strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), SHA256: hex.EncodeToString(sum[:])}
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			return nil, err
		}
		name := strings.ToUpper(path.Clean(strings.ReplaceAll(f.Name, "\\", "/")))
		if strings.HasPrefix(name, "BOOTSECTOR/") {
			if path.Base(name) == "IP.BIN" {
				if len(b) != 32768 {
					return nil, errors.New("the patch's IP.BIN is not 32 KB")
				}
				d.IPBIN = b
			}
			continue
		}
		if strings.HasSuffix(name, ".XDELTA") {
			d.Files = append(d.Files, dcpFile{Path: strings.TrimSuffix(name, ".XDELTA"), Delta: b})
		} else {
			d.Files = append(d.Files, dcpFile{Path: name, Whole: b})
		}
	}
	if d.IPBIN == nil && len(d.Files) == 0 {
		return nil, errors.New("the patch is empty")
	}
	return d, nil
}

// one sector to write: user data only; the raw sector is rebuilt around it
type sectorWrite struct {
	LBA  int
	Data []byte // 2048 bytes
}

type patchReport struct {
	Folder   string   `json:"folder"`
	Patch    string   `json:"patch"`
	Changes  []string `json:"changes"`
	Sectors  int      `json:"sectors"`
	UndoFile string   `json:"undo,omitempty"`
	DryRun   bool     `json:"dryRun"`
}

type undoHeader struct {
	Patch      string `json:"patch"`
	SHA256     string `json:"sha256"`
	When       string `json:"when"`
	SectorSize int    `json:"sectorSize"` // raw bytes saved per sector
	LBAs       []int  `json:"lbas"`
}

func patchDir(root, folder string) string {
	return filepath.Join(root, backupDir, "patches", filepath.Base(folder))
}

// trackFor finds the data track holding an LBA.
func trackFor(tracks []gdiTrack, lba int) (gdiTrack, bool) {
	for _, t := range tracks {
		if t.Type == 4 && lba >= t.LBA && lba < t.LBA+t.Sectors {
			return t, true
		}
	}
	return gdiTrack{}, false
}

// planDCP works out every sector the patch changes without writing anything.
func planDCP(folder string, d *dcpPatch) ([]sectorWrite, []string, []gdiTrack, error) {
	gdiPath := filepath.Join(folder, "disc.gdi")
	if _, err := os.Stat(gdiPath); err != nil {
		return nil, nil, nil, errors.New("only GDI images can be patched in place; a CDI needs Universal Dreamcast Patcher")
	}
	tracks, err := parseGDI(gdiPath)
	if err != nil {
		return nil, nil, nil, err
	}
	disc, err := openGDI(gdiPath)
	if err != nil {
		return nil, nil, nil, err
	}
	defer disc.Close()
	for _, t := range disc.tracks {
		if t.SectorSize != 2048 && t.SectorSize != 2352 {
			return nil, nil, nil, fmt.Errorf("track %d has %d byte sectors, which SWIRL cannot patch", t.Num, t.SectorSize)
		}
	}
	var writes []sectorWrite
	var changes []string
	base := disc.highDensityStart()
	if d.IPBIN != nil {
		cur, err := disc.readSectors(base, 16)
		if err != nil {
			return nil, nil, nil, err
		}
		n := 0
		for i := 0; i < 16; i++ {
			if !bytes.Equal(cur[i*sectorSize:(i+1)*sectorSize], d.IPBIN[i*sectorSize:(i+1)*sectorSize]) {
				writes = append(writes, sectorWrite{LBA: base + i, Data: d.IPBIN[i*sectorSize : (i+1)*sectorSize]})
				n++
			}
		}
		if n > 0 {
			changes = append(changes, fmt.Sprintf("IP.BIN: %d of 16 sectors", n))
		} else {
			changes = append(changes, "IP.BIN: already as the patch wants it")
		}
	}
	if len(d.Files) > 0 {
		files, err := listISO(disc)
		if err != nil {
			return nil, nil, nil, err
		}
		byPath := map[string]isoFile{}
		for _, f := range files {
			if !f.Dir {
				byPath[strings.ToUpper(f.Path)] = f
			}
		}
		for _, pf := range d.Files {
			f, ok := byPath[pf.Path]
			if !ok {
				return nil, nil, nil, fmt.Errorf("%s is not on this disc; the patch is for another game or version", pf.Path)
			}
			nsec := (f.Size + sectorSize - 1) / sectorSize
			raw, err := disc.readSectors(f.LBA, nsec)
			if err != nil {
				return nil, nil, nil, err
			}
			orig := raw[:f.Size]
			var next []byte
			if pf.Delta != nil {
				next, err = vcdiffApply(pf.Delta, orig)
				if err != nil {
					return nil, nil, nil, fmt.Errorf("%s: %v", pf.Path, err)
				}
			} else {
				next = pf.Whole
			}
			if len(next) != f.Size {
				return nil, nil, nil, fmt.Errorf("%s: the patch changes the file's size (%d to %d bytes); that needs a rebuilt image (Universal Dreamcast Patcher)", pf.Path, f.Size, len(next))
			}
			n := 0
			for i := 0; i < nsec; i++ {
				lo, hi := i*sectorSize, (i+1)*sectorSize
				if hi > f.Size {
					hi = f.Size
				}
				if !bytes.Equal(orig[lo:hi], next[lo:hi]) {
					data := append([]byte(nil), raw[i*sectorSize:(i+1)*sectorSize]...)
					copy(data, next[lo:hi])
					writes = append(writes, sectorWrite{LBA: f.LBA + i, Data: data})
					n++
				}
			}
			if n > 0 {
				changes = append(changes, fmt.Sprintf("%s: %d of %d sectors", pf.Path, n, nsec))
			} else {
				changes = append(changes, fmt.Sprintf("%s: already patched", pf.Path))
			}
		}
	}
	sort.Slice(writes, func(i, j int) bool { return writes[i].LBA < writes[j].LBA })
	return writes, changes, tracks, nil
}

// applyDCP patches one game folder. With dry set nothing is written and the report says what would change.
func applyDCP(root, folder, dcpPath string, dry bool) (*patchReport, error) {
	d, err := readDCP(dcpPath)
	if err != nil {
		return nil, err
	}
	if !dry {
		unlock, err := lockCard(root, "VGA patch")
		if err != nil {
			return nil, err
		}
		defer unlock()
	}
	writes, changes, tracks, err := planDCP(folder, d)
	if err != nil {
		return nil, err
	}
	rep := &patchReport{Folder: filepath.Base(folder), Patch: d.Name, Changes: changes, Sectors: len(writes), DryRun: dry}
	if dry || len(writes) == 0 {
		return rep, nil
	}
	// the original raw sectors, saved before anything is written
	rawSize := 0
	for _, w := range writes {
		t, ok := trackFor(tracks, w.LBA)
		if !ok {
			return nil, fmt.Errorf("LBA %d is not in a data track", w.LBA)
		}
		if rawSize == 0 {
			rawSize = t.SectorSize
		} else if rawSize != t.SectorSize {
			return nil, errors.New("the patch spans tracks with different sector sizes")
		}
	}
	undoDir := patchDir(root, folder)
	if err := os.MkdirAll(undoDir, 0o755); err != nil {
		return nil, err
	}
	hdr := undoHeader{Patch: d.Name, SHA256: d.SHA256, When: time.Now().Format(time.RFC3339), SectorSize: rawSize}
	var saved bytes.Buffer
	for _, w := range writes {
		t, _ := trackFor(tracks, w.LBA)
		f, err := os.Open(t.File)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, rawSize)
		_, err = f.ReadAt(buf, int64(w.LBA-t.LBA)*int64(rawSize))
		f.Close()
		if err != nil {
			return nil, err
		}
		if rawSize == 2352 && !sectorIsMode1(buf) {
			return nil, fmt.Errorf("sector %d is not a mode 1 data sector; the image is not one SWIRL can patch", w.LBA)
		}
		hdr.LBAs = append(hdr.LBAs, w.LBA)
		saved.Write(buf)
	}
	hb, _ := json.Marshal(hdr)
	undoPath := filepath.Join(undoDir, safeFileName(d.Name)+".undo")
	if err := writeCardFile(undoPath, append(append(hb, '\n'), saved.Bytes()...)); err != nil {
		return nil, fmt.Errorf("could not save the undo file: %v", err)
	}
	rep.UndoFile = undoPath
	if err := writeSectors(tracks, writes, rawSize); err != nil {
		return nil, err
	}
	os.Remove(filepath.Join(undoDir, "skip")) // a patch applied on purpose ends an earlier "leave it"
	note := fmt.Sprintf("%s %s %s\r\n", hdr.When, d.SHA256[:16], d.Name)
	if err := appendCardFile(filepath.Join(folder, "patches.txt"), note); err != nil {
		return nil, err
	}
	return rep, nil
}

func safeFileName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 32 || strings.ContainsRune(`<>:"/\|?*`, r) {
			r = '_'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// writeSectors writes user data into the track files, rebuilding raw sectors around it, and syncs each file.
func writeSectors(tracks []gdiTrack, writes []sectorWrite, rawSize int) error {
	open := map[string]*os.File{}
	defer func() {
		for _, f := range open {
			f.Close()
		}
	}()
	for _, w := range writes {
		t, _ := trackFor(tracks, w.LBA)
		f := open[t.File]
		if f == nil {
			var err error
			if f, err = os.OpenFile(t.File, os.O_RDWR, 0); err != nil {
				return err
			}
			open[t.File] = f
		}
		off := int64(w.LBA-t.LBA) * int64(rawSize)
		var out []byte
		if rawSize == 2352 {
			out = make([]byte, 2352)
			if _, err := f.ReadAt(out, off); err != nil {
				return err
			}
			copy(out[16:], w.Data)
			sectorFixMode1(out)
		} else {
			out = w.Data
		}
		if _, err := f.WriteAt(out, off); err != nil {
			return err
		}
	}
	for _, f := range open {
		if err := f.Sync(); err != nil {
			return err
		}
	}
	return nil
}

func appendCardFile(p, s string) error {
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(s); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// undoDCP puts the saved sectors back. The undo file is kept (renamed .undone) as a record.
func undoDCP(root, folder, undoPath string) (*patchReport, error) {
	unlock, err := lockCard(root, "VGA patch undo")
	if err != nil {
		return nil, err
	}
	defer unlock()
	raw, err := os.ReadFile(undoPath)
	if err != nil {
		return nil, err
	}
	nl := bytes.IndexByte(raw, '\n')
	if nl < 0 {
		return nil, errors.New("the undo file is damaged")
	}
	var hdr undoHeader
	if err := json.Unmarshal(raw[:nl], &hdr); err != nil {
		return nil, errors.New("the undo file is damaged")
	}
	body := raw[nl+1:]
	if len(body) != len(hdr.LBAs)*hdr.SectorSize {
		return nil, errors.New("the undo file is cut short")
	}
	tracks, err := parseGDI(filepath.Join(folder, "disc.gdi"))
	if err != nil {
		return nil, err
	}
	open := map[string]*os.File{}
	defer func() {
		for _, f := range open {
			f.Close()
		}
	}()
	for i, lba := range hdr.LBAs {
		t, ok := trackFor(tracks, lba)
		if !ok || t.SectorSize != hdr.SectorSize {
			return nil, fmt.Errorf("the disc no longer matches the undo file at sector %d", lba)
		}
		f := open[t.File]
		if f == nil {
			if f, err = os.OpenFile(t.File, os.O_RDWR, 0); err != nil {
				return nil, err
			}
			open[t.File] = f
		}
		if _, err := f.WriteAt(body[i*hdr.SectorSize:(i+1)*hdr.SectorSize], int64(lba-t.LBA)*int64(hdr.SectorSize)); err != nil {
			return nil, err
		}
	}
	for _, f := range open {
		if err := f.Sync(); err != nil {
			return nil, err
		}
	}
	_ = os.Rename(undoPath, undoPath+".undone")
	// a patch taken off on purpose stays off: the automatic pass leaves this folder alone from now on
	_ = os.WriteFile(filepath.Join(filepath.Dir(undoPath), "skip"), []byte("The VGA patch was removed by hand; Card Manager will not apply it again by itself.\r\n"), 0o644)
	_ = appendCardFile(filepath.Join(folder, "patches.txt"), fmt.Sprintf("%s removed %s\r\n", time.Now().Format(time.RFC3339), hdr.Patch))
	return &patchReport{Folder: filepath.Base(folder), Patch: hdr.Patch, Sectors: len(hdr.LBAs), Changes: []string{"restored from " + filepath.Base(undoPath)}}, nil
}

// ---------- the VGA patch catalog ----------

// A catalog entry names a game by product and version and points at a .dcp the community made for it.
type vgaPatchEntry struct {
	Product string `json:"product"` // as in IP.BIN, for example T-9702N
	Version string `json:"version"` // as in IP.BIN, for example V1.020
	Name    string `json:"name"`
	Author  string `json:"author"`
	URL     string `json:"url"`
	SHA256  string `json:"sha256"`
	Notes   string `json:"notes,omitempty"`
}

func productKey(s string) string {
	return strings.ToUpper(strings.NewReplacer("-", "", " ", "").Replace(strings.TrimSpace(s)))
}

func findVGAPatch(catalog []vgaPatchEntry, product, version string) *vgaPatchEntry {
	p := productKey(product)
	v := strings.ToUpper(strings.TrimSpace(version))
	for i := range catalog {
		if productKey(catalog[i].Product) == p && (catalog[i].Version == "" || strings.ToUpper(catalog[i].Version) == v) {
			return &catalog[i]
		}
	}
	return nil
}

// checkDCPHash refuses a downloaded patch whose contents are not the ones the catalog lists.
func checkDCPHash(p, want string) error {
	if want == "" {
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), want) {
		return errors.New("the downloaded patch does not match the catalog; it was not applied")
	}
	return nil
}

var _ = binary.LittleEndian

// vgaUndoFile is the undo file of the patch applied to a folder, or "" when none is applied.
func vgaUndoFile(root, folder string) string {
	m, _ := filepath.Glob(filepath.Join(patchDir(root, folder), "*.undo"))
	if len(m) == 0 {
		return ""
	}
	sort.Strings(m)
	return m[len(m)-1]
}

// vgaSkipped reports whether the owner took the patch off this folder by hand.
func vgaSkipped(root, folder string) bool {
	_, err := os.Stat(filepath.Join(patchDir(root, folder), "skip"))
	return err == nil
}
