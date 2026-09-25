package main

// SD card logic: scan game folders, rebuild the menu disc in folder 01, backups and restore.

import (
	"bytes"
	"crypto/sha1"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed assets/1ST_READ.BIN
var swirlBinary []byte

//go:embed assets/IP.BIN
var fallbackIP []byte

const backupDir = "SWIRL_BACKUP"

type Game struct {
	Folder  string `json:"folder"`
	Slot    int    `json:"slot"`
	Name    string `json:"name"`
	Custom  bool   `json:"custom"` // name comes from name.txt
	Product string `json:"product"`
	Region  string `json:"region"`
	Disc    string `json:"disc"`
	Version string `json:"version"`
	Date    string `json:"date"`
	VGA     bool   `json:"vga"`
	Format  string `json:"format"`
	Type    string `json:"type,omitempty"` // "psx" for PlayStation discs (run through Bleem)
	HasArt  bool   `json:"hasArt"`
	HasMeta bool   `json:"hasMeta"`
	HasVMU  bool   `json:"hasVmu"`
	Edited  bool   `json:"edited"`
	Shots   int    `json:"shots"`
	Error   string `json:"error,omitempty"`
	// proper name when the current one could be better ("" otherwise)
	Suggested string `json:"suggested,omitempty"`
	UserName  bool   `json:"userName,omitempty"`
	// multi disc games: every disc of a set shares Set; DiscNo and DiscOf come from the disc header
	Set    string `json:"set,omitempty"`
	DiscNo int    `json:"discNo,omitempty"`
	DiscOf int    `json:"discOf,omitempty"`
}

type Card struct {
	Root      string `json:"root"`
	MenuType  string `json:"menuType"`
	MenuTitle string `json:"menuTitle"`
	SwirlVer  string `json:"swirlVersion"`
	// SwirlRelease is the version SWIRL reports in About ("2.10"); empty for early builds
	SwirlRelease string     `json:"swirlRelease,omitempty"`
	HasBox       bool       `json:"hasBox"`
	HasIcon      bool       `json:"hasIcon"`
	HasMeta      bool       `json:"hasMeta"`
	Games        []Game     `json:"games"`
	Warnings     []string   `json:"warnings"`
	Backups      []string   `json:"backups"`
	Dups         []DupGroup `json:"duplicates"`
	Sets         []DiscSet  `json:"sets"`
}

var folderRe = regexp.MustCompile(`^\d{2,3}$`)

func swirlHash(b []byte) string {
	h := sha1.Sum(b)
	return fmt.Sprintf("%x", h[:4])
}

func openMenuProduct(serial string) string {
	s := strings.ReplaceAll(serial, "-", "")
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return s
}

// datIDs reads the ID table of a DAT file held inside the menu disc.
func datIDs(d sectorReader, f isoFile) map[string]bool {
	ids := map[string]bool{}
	hdr, err := d.readSectors(f.LBA, 1)
	if err != nil || string(hdr[0:3]) != "DAT" {
		return ids
	}
	n := int(binary.LittleEndian.Uint32(hdr[8:]))
	if n <= 0 || n > 100000 {
		return ids
	}
	b, err := d.readSectors(f.LBA, (16+16*n+sectorSize-1)/sectorSize)
	if err != nil {
		return ids
	}
	for i := 0; i < n; i++ {
		rec := b[16+16*i : 16+16*i+12]
		if j := bytes.IndexByte(rec, 0); j >= 0 {
			rec = rec[:j]
		}
		ids[string(rec)] = true
	}
	return ids
}

func readText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func listBackups(root string) []string {
	var out []string
	entries, _ := os.ReadDir(filepath.Join(root, backupDir))
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(out)))
	return out
}

func ScanCard(root string) (*Card, error) {
	st, err := os.Stat(root)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("%s is not a folder", root)
	}
	c := &Card{Root: root, MenuType: "None", Backups: listBackups(root)}
	var boxIDs, iconIDs, metaIDs, vmuIDs map[string]bool
	edits := loadEdits(root)

	// menu disc in 01
	gdi := findGDI(filepath.Join(root, "01"))
	if gdi != "" {
		if d, err := openGDI(gdi); err == nil {
			if b, err := d.readSectors(d.highDensityStart(), 1); err == nil {
				if ip, ok := parseIP(b); ok {
					c.MenuTitle = ip.Name
					switch {
					case strings.Contains(ip.Name, "openMenu"):
						c.MenuType = "openMenu"
					case strings.Contains(strings.ToUpper(ip.Name), "GDMENU"):
						c.MenuType = "GDMENU"
					default:
						c.MenuType = "Game"
					}
				}
			}
			if c.MenuType == "openMenu" {
				if files, err := listISO(d); err == nil {
					for _, f := range files {
						switch strings.ToUpper(f.Path) {
						case "BOX.DAT":
							c.HasBox = true
							boxIDs = datIDs(d, f)
						case "ICON.DAT":
							c.HasIcon = true
							iconIDs = datIDs(d, f)
						case "META.DAT":
							c.HasMeta = true
							metaIDs = datIDs(d, f)
						case "VMU.DAT":
							vmuIDs = datIDs(d, f)
						case "1ST_READ.BIN":
							if b, err := d.readSectors(f.LBA, (f.Size+sectorSize-1)/sectorSize); err == nil {
								b = b[:f.Size]
								if bytes.Contains(b, []byte("SWIRL: %d games")) {
									c.MenuType = "SWIRL"
									c.SwirlVer = swirlHash(b)
									c.SwirlRelease = swirlRelease(b)
								}
							}
						}
					}
				}
			}
			d.Close()
		}
	}
	if c.MenuType == "Game" {
		c.Warnings = append(c.Warnings, "Folder 01 holds a game, not a menu. Move it to a later folder first.")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var nums []int
	for _, e := range entries {
		if !e.IsDir() || !folderRe.MatchString(e.Name()) {
			continue
		}
		n, _ := strconv.Atoi(e.Name())
		if n <= 1 {
			continue
		}
		nums = append(nums, n)
	}
	sort.Ints(nums)
	gapWarned := false
	for i, n := range nums {
		folder := fmt.Sprintf("%02d", n)
		if _, err := os.Stat(filepath.Join(root, folder)); err != nil {
			folder = strconv.Itoa(n)
		}
		g := Game{Folder: folder, Slot: n, Disc: "1/1", Region: "JUE", Version: "V1.000", Date: "20000101", VGA: true}
		dir := filepath.Join(root, folder)
		ip, format, err := readImageIP(dir)
		g.Format = format
		if err != nil {
			g.Error = err.Error()
		}
		if ip != nil {
			g.Name, g.Product, g.Region, g.Disc, g.Version, g.Date, g.VGA = ip.Name, openMenuProduct(ip.Product), ip.Region, ip.Disc, ip.Version, ip.Date, ip.VGA
		} else if serial, ok := detectPSX(dir); ok {
			g.Type, g.Format, g.Error, g.Region = "psx", "PSX", "", "U"
			g.Product = serial
			g.Name = folder
		}
		if s := readText(filepath.Join(dir, "serial.txt")); s != "" {
			g.Product = openMenuProduct(s)
		}
		if name := readText(filepath.Join(dir, "name.txt")); name != "" {
			g.Name, g.Custom = name, true
		}
		if g.Name == "" {
			g.Name = "Folder " + folder
		}
		if e := edits.Games[folder]; e != nil && e.Product == g.Product {
			g.Edited = true
			if e.Region != "" {
				g.Region = e.Region
			}
			if e.VGA != nil {
				g.VGA = *e.VGA
			}
			if e.Date != "" {
				g.Date = e.Date
			}
			if e.Meta != nil {
				g.HasMeta = true
			}
		}
		mine := edits.Games[folder] != nil && edits.Games[folder].Product == g.Product
		g.HasArt = iconIDs[g.Product] || boxIDs[g.Product] || (mine && fileExists(artPath(root, folder, "box")))
		g.HasMeta = g.HasMeta || metaIDs[g.Product]
		g.HasVMU = vmuIDs[g.Product] || (mine && fileExists(artPath(root, folder, "vmu")))
		g.Shots = len(shotsFor(root, g.Product))
		c.Games = append(c.Games, g)
		if n != i+2 && !gapWarned {
			gapWarned = true
			c.Warnings = append(c.Warnings, fmt.Sprintf("Folder numbers skip a number before %s. GDEMU expects 02, 03, 04 ... with no gaps; renumber with GDMENUCardManager.", folder))
		}
	}
	for i := range c.Games {
		g := &c.Games[i]
		g.DiscNo, g.DiscOf = discNumbers(g.Disc)
		if e := edits.Games[g.Folder]; e != nil && e.Product == g.Product && e.UserName {
			g.UserName = true
			continue
		}
		if sg := suggestedName(g); sg != "" && sg != g.Name {
			g.Suggested = sg
		}
	}
	c.Sets = findDiscSets(c.Games)
	c.Dups = findDuplicates(root, c.Games)
	// empty lists, not null, for the web page (a new card has no games yet)
	if c.Games == nil {
		c.Games = []Game{}
	}
	if c.Warnings == nil {
		c.Warnings = []string{}
	}
	if c.Backups == nil {
		c.Backups = []string{}
	}
	if c.Dups == nil {
		c.Dups = []DupGroup{}
	}
	if c.Sets == nil {
		c.Sets = []DiscSet{}
	}
	return c, nil
}

func findGDI(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.EqualFold(filepath.Ext(e.Name()), ".gdi") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func iniEntry(b *strings.Builder, slot int, name, disc string, vga bool, region, version, date, product string, extra ...string) {
	v := "0"
	if vga {
		v = "1"
	}
	s := fmt.Sprintf("%02d", slot)
	fmt.Fprintf(b, "%s.name=%s\r\n%s.disc=%s\r\n%s.vga=%s\r\n%s.region=%s\r\n%s.version=%s\r\n%s.date=%s\r\n%s.product=%s\r\n\r\n",
		s, name, s, disc, s, v, s, region, s, version, s, date, s, product)
	if len(extra) > 0 && extra[0] != "" {
		// the type line goes with the entry (SWIRL reads it; stock openMenu ignores unknown keys)
		out := b.String()
		out = strings.TrimSuffix(out, "\r\n")
		b.Reset()
		b.WriteString(out)
		fmt.Fprintf(b, "%s.type=%s\r\n\r\n", s, extra[0])
	}
}

func buildINI(c *Card, menuIP *ipInfo) string {
	var b strings.Builder
	max := 1
	for _, g := range c.Games {
		if g.Slot > max {
			max = g.Slot
		}
	}
	fmt.Fprintf(&b, "[OPENMENU]\r\nnum_items=%d\r\n\r\n[ITEMS]\r\n", max)
	iniEntry(&b, 1, "openMenu", "1/1", true, "JUE", menuIP.Version, menuIP.Date, openMenuProduct(menuIP.Product))
	for _, g := range c.Games {
		name := strings.ReplaceAll(strings.ReplaceAll(g.Name, "\r", " "), "\n", " ")
		iniEntry(&b, g.Slot, name, g.Disc, g.VGA, g.Region, g.Version, g.Date, g.Product, g.Type)
	}
	return b.String()
}

type Logger func(format string, args ...any)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// InstallSwirl rebuilds 01 with SWIRL, keeping the existing art, metadata and themes. Game folders are not touched.
func InstallSwirl(root, datDir string, log Logger) error {
	// a card that already has a menu in 01 may have no games yet (a new card); anything else needs games,
	// so SWIRL is not put on the wrong drive by mistake
	st, err := os.Stat(filepath.Join(root, "01"))
	return installSwirl(root, datDir, err == nil && st.IsDir(), log)
}

// installSwirl builds the menu. allowEmpty is used for a freshly formatted card with no games yet.
// buildMenuImage builds the complete menu disc for a card into a temporary folder (returned as out;
// remove work when done). The card itself is not changed.
func buildMenuImage(root, datDir string, allowEmpty bool, log Logger) (work, out string, c *Card, err error) {
	return buildMenuImageInto(root, datDir, allowEmpty, log)
}

func buildMenuImageInto(root, datDir string, allowEmpty bool, log Logger) (string, string, *Card, error) {
	c, err := ScanCard(root)
	if err != nil {
		return "", "", nil, err
	}
	if c.MenuType == "Game" {
		return "", "", nil, errors.New("folder 01 holds a game, not a menu; nothing was changed")
	}
	if len(c.Games) == 0 && !allowEmpty {
		return "", "", nil, errors.New("no game folders (02, 03, ...) were found on this card")
	}
	log("Found %d games", len(c.Games))

	work, err := os.MkdirTemp("", "swirl_")
	if err != nil {
		return "", "", nil, err
	}
	data := filepath.Join(work, "data")
	low := filepath.Join(work, "low")
	out := filepath.Join(work, "out")
	for _, p := range []string{data, low, out} {
		os.MkdirAll(p, 0o755)
	}

	menuIP := &ipInfo{Name: "openMenu", Version: "V1.000", Date: time.Now().Format("20060102"), Product: "SWIRL_1"}
	ipBytes := fallbackIP
	menuDir := filepath.Join(root, "01")
	if gdi := findGDI(menuDir); gdi != "" && (c.MenuType == "openMenu" || c.MenuType == "SWIRL") {
		d, err := openGDI(gdi)
		if err != nil {
			return work, "", nil, err
		}
		if b, err := d.readSectors(d.highDensityStart(), 16); err == nil {
			if ip, ok := parseIP(b); ok {
				ipBytes, menuIP = b, ip
			}
		}
		files, err := listISO(d)
		if err != nil {
			d.Close()
			return work, "", nil, err
		}
		n := 0
		for _, f := range files {
			if f.Dir || strings.EqualFold(f.Path, "1ST_READ.BIN") || strings.EqualFold(f.Path, "OPENMENU.INI") {
				continue
			}
			if err := extractISOFile(d, f, filepath.Join(data, filepath.FromSlash(f.Path))); err != nil {
				d.Close()
				return work, "", nil, fmt.Errorf("reading %s from the current menu: %w", f.Path, err)
			}
			n++
		}
		d.Close()
		log("Kept %d files from the current menu (art, metadata, themes)", n)
	} else {
		log("No openMenu disc in 01 yet; building a fresh menu")
	}
	if datDir != "" {
		for _, name := range []string{"BOX.DAT", "ICON.DAT", "META.DAT", "BOX_EX.DAT", "ICON_EX.DAT"} {
			src := filepath.Join(datDir, name)
			if _, err := os.Stat(src); err != nil {
				// try lower case
				src = filepath.Join(datDir, strings.ToLower(name))
				if _, err := os.Stat(src); err != nil {
					continue
				}
			}
			// remove any existing copy with different case
			entries, _ := os.ReadDir(data)
			for _, e := range entries {
				if strings.EqualFold(e.Name(), name) {
					os.Remove(filepath.Join(data, e.Name()))
				}
			}
			if err := copyFile(src, filepath.Join(data, name)); err != nil {
				return work, "", nil, err
			}
			log("Imported %s", name)
		}
	}
	if err := mergeOnlineDB(data, c, log); err != nil {
		log("Online art skipped: %v", err)
	}
	if err := applyEdits(root, c, data, log); err != nil {
		return work, "", nil, err
	}
	if err := addExtras(root, c, data, log); err != nil {
		return work, "", nil, err
	}
	bin := swirlBinary
	if p := os.Getenv("SWIRL_1ST_READ"); p != "" { // development: test a new menu build
		// only builds that keep the author credit may be installed
		if b, err := os.ReadFile(p); err == nil && bytes.Contains(b, []byte("Created by Glen Huszar")) {
			bin = b
		}
	}
	if err := os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), bin, 0o644); err != nil {
		return work, "", nil, err
	}
	ini := buildINI(c, menuIP)
	os.WriteFile(filepath.Join(data, "OPENMENU.INI"), []byte(ini), 0o644)
	os.WriteFile(filepath.Join(low, "OPENMENU.INI"), []byte(ini), 0o644)
	os.WriteFile(filepath.Join(low, "GDEMUNFO.TXT"), []byte("Generated using SWIRL Card Manager"), 0o644)

	log("Building the menu disc")
	if err := buildMenuDisc(data, low, out, ipBytes); err != nil {
		return work, "", nil, err
	}
	return work, out, c, nil
}

func installSwirl(root, datDir string, allowEmpty bool, log Logger) error {
	work, out, c, err := buildMenuImage(root, datDir, allowEmpty, log)
	if work != "" {
		defer os.RemoveAll(work)
	}
	if err != nil {
		return err
	}
	_ = c
	menuDir := filepath.Join(root, "01")
	// back up the current 01, then write the new one
	backup := ""
	if entries, err := os.ReadDir(menuDir); err == nil && len(entries) > 0 {
		backup = filepath.Join(root, backupDir, "01_"+time.Now().Format("20060102_150405"))
		if err := os.MkdirAll(backup, 0o755); err != nil {
			return err
		}
		for _, e := range entries {
			if err := os.Rename(filepath.Join(menuDir, e.Name()), filepath.Join(backup, e.Name())); err != nil {
				return fmt.Errorf("backing up 01: %w", err)
			}
		}
		log("Backed up the old menu to %s", filepath.Join(backupDir, filepath.Base(backup)))
	}
	os.MkdirAll(menuDir, 0o755)
	for _, name := range []string{"disc.gdi", "track01.iso", "track02.raw", "track03.iso", "track04.raw", "track05.iso"} {
		if err := copyFile(filepath.Join(out, name), filepath.Join(menuDir, name)); err != nil {
			// put the backup back
			if backup != "" {
				entries, _ := os.ReadDir(menuDir)
				for _, e := range entries {
					os.Remove(filepath.Join(menuDir, e.Name()))
				}
				restoreFrom(backup, menuDir)
			}
			return fmt.Errorf("writing to the SD card failed, old menu restored: %w", err)
		}
	}
	pruneMenuBackups(root, 3)
	log("SWIRL installed in folder 01. Game folders were not changed.")
	return nil
}

func restoreFrom(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.Rename(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return os.Remove(src)
}

// RestoreBackup puts a saved menu back into 01, keeping the current one as another backup.
func RestoreBackup(root, name string, log Logger) error {
	if strings.ContainsAny(name, `/\`) || name == "" || name == "." || name == ".." {
		return errors.New("bad backup name")
	}
	if strings.HasPrefix(name, "removed_") {
		return errors.New("that folder holds removed game copies, not a menu")
	}
	src := filepath.Join(root, backupDir, name)
	if _, err := os.Stat(src); err != nil {
		return err
	}
	menuDir := filepath.Join(root, "01")
	if entries, err := os.ReadDir(menuDir); err == nil && len(entries) > 0 {
		keep := filepath.Join(root, backupDir, "01_replaced_"+time.Now().Format("20060102_150405"))
		os.MkdirAll(keep, 0o755)
		for _, e := range entries {
			if err := os.Rename(filepath.Join(menuDir, e.Name()), filepath.Join(keep, e.Name())); err != nil {
				return err
			}
		}
		log("Current menu saved as %s", filepath.Base(keep))
	}
	os.MkdirAll(menuDir, 0o755)
	if err := restoreFrom(src, menuDir); err != nil {
		return err
	}
	log("Restored %s into folder 01", name)
	return nil
}

// SaveNames writes name.txt files (the same convention GDMENUCardManager uses).
func SaveNames(root string, names map[string]string, log Logger) error {
	for folder, name := range names {
		if !folderRe.MatchString(folder) {
			continue
		}
		name = strings.TrimSpace(name)
		p := filepath.Join(root, folder, "name.txt")
		if name == "" {
			os.Remove(p)
			continue
		}
		if err := os.WriteFile(p, []byte(name), 0o644); err != nil {
			return err
		}
		log("Renamed %s to %s", folder, name)
	}
	return nil
}

// gdEnd is the last LBA of a GD-ROM high density area. Menu data must end here or the Dreamcast
// BIOS will not boot the disc (it drops to the BIOS menu).
const gdEnd = 549150

// buildMenuDisc writes a 5 track GDI laid out exactly like GDMENUCardManager's menu discs:
// track01 low density ISO (OPENMENU.INI), track03 at 45000 with IP.BIN and the ISO directories,
// track05 with the file data ending at LBA 549150, and 1 sector audio tracks 02 and 04.
func buildMenuDisc(dataDir, lowDir, outDir string, ipIn []byte) error {
	if len(ipIn) < 0x8000 {
		return errors.New("IP.BIN is too short")
	}
	ip := make([]byte, 0x8000)
	copy(ip, ipIn[:0x8000])
	boot := strings.TrimSpace(string(bytes.TrimRight(ip[0x60:0x70], "\x00")))
	if boot == "" {
		boot = "1ST_READ.BIN"
	}
	// first pass finds where the data track starts, then IP.BIN gets the matching track table
	start, err := buildISOSplit(dataDir, filepath.Join(outDir, "track03.iso"), filepath.Join(outDir, "track05.iso"), 45000, gdEnd, "OPENMENU", ip, boot)
	if err != nil {
		return err
	}
	patchIPTOC(ip, []ipTrack{{45000, 4}, {start - 151, 0}, {start, 4}})
	f, err := os.OpenFile(filepath.Join(outDir, "track03.iso"), os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	_, err = f.WriteAt(ip, 0)
	f.Close()
	if err != nil {
		return err
	}
	if err := buildISO(lowDir, filepath.Join(outDir, "track01.iso"), 0, "OPENMENU", nil); err != nil {
		return err
	}
	audio := make([]byte, 2352)
	os.WriteFile(filepath.Join(outDir, "track02.raw"), audio, 0o644)
	os.WriteFile(filepath.Join(outDir, "track04.raw"), audio, 0o644)
	gdi := fmt.Sprintf("5\r\n1 0 4 2048 track01.iso 0\r\n2 450 0 2352 track02.raw 0\r\n3 45000 4 2048 track03.iso 0\r\n4 %d 0 2352 track04.raw 0\r\n5 %d 4 2048 track05.iso 0\r\n", start-151, start)
	return os.WriteFile(filepath.Join(outDir, "disc.gdi"), []byte(gdi), 0o644)
}

// buildTestMenu builds a menu disc from a plain folder (1ST_READ.BIN, OPENMENU.INI, DATs, themes).
// The embedded SWIRL binary is used when the folder has no 1ST_READ.BIN.
func buildTestMenu(dataDir, outDir string) error {
	if outDir == "" {
		return errors.New("-out is required")
	}
	work, err := os.MkdirTemp("", "swirl_")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	data, low := filepath.Join(work, "data"), filepath.Join(work, "low")
	os.MkdirAll(low, 0o755)
	if err := copyTree(dataDir, data); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(data, "1ST_READ.BIN")); err != nil {
		os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), swirlBinary, 0o644)
	}
	if err := copyFile(filepath.Join(data, "OPENMENU.INI"), filepath.Join(low, "OPENMENU.INI")); err != nil {
		return err
	}
	os.WriteFile(filepath.Join(low, "GDEMUNFO.TXT"), []byte("Generated using SWIRL Card Manager"), 0o644)
	os.MkdirAll(outDir, 0o755)
	for _, n := range []string{"disc.gdi", "track01.iso", "track02.raw", "track03.iso", "track04.raw", "track05.iso"} {
		os.Remove(filepath.Join(outDir, n))
	}
	return buildMenuDisc(data, low, outDir, fallbackIP)
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if info.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
}

type ipTrack struct{ lba, typ int }

// patchIPTOC writes the high density track table into IP.BIN at 0x104. The Dreamcast BIOS compares it
// with the real disc and refuses to boot (drops to the BIOS menu) when they differ.
func patchIPTOC(ip []byte, tracks []ipTrack) {
	for t := 0; t < 97; t++ {
		lba, typ := uint32(0xFFFFFF), byte(0xFF)
		if t < len(tracks) {
			lba = uint32(tracks[t].lba + 150)
			typ = byte(tracks[t].typ<<4 | 1)
		}
		o := 0x104 + t*4
		ip[o], ip[o+1], ip[o+2], ip[o+3] = byte(lba), byte(lba>>8), byte(lba>>16), typ
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// findDataFile returns the path of name in dir, matching case insensitively.
func findDataFile(dir, name string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.EqualFold(e.Name(), name) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func loadOrNewDat(dir, name string, chunk int) (*datFile, string) {
	p := findDataFile(dir, name)
	if p != "" {
		if d, err := readDat(p); err == nil {
			return d, p
		}
		os.Remove(p)
	}
	return newDat(chunk), filepath.Join(dir, name)
}

// applyEdits merges the SWIRL/ edits into META.DAT, BOX.DAT, ICON.DAT and VMU.DAT in the menu data folder.
func applyEdits(root string, c *Card, data string, log Logger) error {
	edits := loadEdits(root)
	var metaSet, boxSet, vmuSet []*Game
	for i := range c.Games {
		g := &c.Games[i]
		if e := edits.Games[g.Folder]; e != nil && e.Product == g.Product && e.Meta != nil {
			metaSet = append(metaSet, g)
		}
		if e := edits.Games[g.Folder]; e == nil || e.Product != g.Product {
			continue // art saved for a different game that used to live in this folder
		}
		if fileExists(artPath(root, g.Folder, "box")) {
			boxSet = append(boxSet, g)
		}
		if fileExists(artPath(root, g.Folder, "vmu")) {
			vmuSet = append(vmuSet, g)
		}
	}
	if len(metaSet) > 0 {
		d, p := loadOrNewDat(data, "META.DAT", metaSize)
		for _, g := range metaSet {
			d.Set(g.Product, encodeMeta(*edits.Games[g.Folder].Meta))
		}
		if err := d.Write(p); err != nil {
			return err
		}
		log("Saved info for %d games", len(metaSet))
	}
	if len(boxSet) > 0 {
		box, bp := loadOrNewDat(data, "BOX.DAT", 131104)
		icon, ip := loadOrNewDat(data, "ICON.DAT", 32800)
		n := 0
		for _, g := range boxSet {
			f, err := os.Open(artPath(root, g.Folder, "box"))
			if err != nil {
				continue
			}
			img, _, err := image.Decode(f)
			f.Close()
			if err != nil {
				continue
			}
			box.Set(g.Product, encodePVR565(img, 256))
			icon.Set(g.Product, encodePVR565(img, 128))
			n++
		}
		if err := box.Write(bp); err != nil {
			return err
		}
		if err := icon.Write(ip); err != nil {
			return err
		}
		log("Saved box art for %d games", n)
	}
	if len(vmuSet) > 0 {
		d, p := loadOrNewDat(data, "VMU.DAT", vmuBytes)
		for _, g := range vmuSet {
			if b, err := os.ReadFile(artPath(root, g.Folder, "vmu")); err == nil && len(b) == vmuBytes {
				d.Set(g.Product, b)
			}
		}
		if err := d.Write(p); err != nil {
			return err
		}
		log("Saved VMU screens for %d games", len(vmuSet))
	}
	return nil
}

// ---------- editor API ----------

type GameDetail struct {
	Game      Game  `json:"game"`
	Meta      Meta  `json:"meta"`
	HasBox    bool  `json:"hasBox"`
	HasVMU    bool  `json:"hasVmu"`
	HasDisc   bool  `json:"hasDiscArt"`
	MetaFound bool  `json:"metaFound"`
	Stamp     int64 `json:"stamp"`
}

func findGame(c *Card, folder string) *Game {
	for i := range c.Games {
		if c.Games[i].Folder == folder {
			return &c.Games[i]
		}
	}
	return nil
}

func GetGameDetail(root, folder string) (*GameDetail, error) {
	c, err := ScanCard(root)
	if err != nil {
		return nil, err
	}
	g := findGame(c, folder)
	if g == nil {
		return nil, errors.New("game folder not found")
	}
	det := &GameDetail{Game: *g, Stamp: time.Now().UnixNano()}
	edits := loadEdits(root)
	if e := edits.Games[folder]; e != nil && e.Product == g.Product && e.Meta != nil {
		det.Meta, det.MetaFound = *e.Meta, true
	} else if m, err := openMenuDisc(root); err == nil {
		if b, err := m.datChunk("META.DAT", g.Product); err == nil {
			det.Meta, det.MetaFound = decodeMeta(b), true
		}
		m.Close()
	}
	det.HasBox = g.HasArt
	det.HasVMU = g.HasVMU
	if b, err := readDiscFile(filepath.Join(root, folder), "0GDTEX.PVR", 4<<20); err == nil {
		_, derr := decodePVR(b)
		det.HasDisc = derr == nil
	}
	return det, nil
}

// GameArt returns a PNG for kind = box | vmu | disc.
func GameArt(root, folder, kind string) ([]byte, error) {
	c, err := ScanCard(root)
	if err != nil {
		return nil, err
	}
	g := findGame(c, folder)
	if g == nil {
		return nil, os.ErrNotExist
	}
	switch kind {
	case "disc":
		b, err := readDiscFile(filepath.Join(root, folder), "0GDTEX.PVR", 4<<20)
		if err != nil {
			return nil, err
		}
		img, err := decodePVR(b)
		if err != nil {
			return nil, err
		}
		return pngBytes(img), nil
	case "box":
		if b, err := os.ReadFile(artPath(root, folder, "box")); err == nil {
			return b, nil
		}
		m, err := openMenuDisc(root)
		if err != nil {
			return nil, err
		}
		defer m.Close()
		b, err := m.datChunk("BOX.DAT", g.Product)
		if err != nil {
			if b, err = m.datChunk("ICON.DAT", g.Product); err != nil {
				return nil, err
			}
		}
		img, err := decodePVR(b)
		if err != nil {
			return nil, err
		}
		return pngBytes(img), nil
	case "vmu":
		if b, err := os.ReadFile(artPath(root, folder, "vmu")); err == nil {
			return vmuPNG(b), nil
		}
		m, err := openMenuDisc(root)
		if err != nil {
			return nil, err
		}
		defer m.Close()
		b, err := m.datChunk("VMU.DAT", g.Product)
		if err != nil {
			return nil, err
		}
		return vmuPNG(b[:vmuBytes]), nil
	}
	return nil, errors.New("unknown art kind")
}

type SaveGameRequest struct {
	Root     string `json:"root"`
	Folder   string `json:"folder"`
	Name     string `json:"name"`
	Serial   string `json:"serial"`
	Region   string `json:"region"`
	VGA      bool   `json:"vga"`
	Date     string `json:"date"`
	Meta     Meta   `json:"meta"`
	Box      string `json:"box"`      // data URL of a new image
	BoxDisc  bool   `json:"boxDisc"`  // use the disc's own artwork (0GDTEX.PVR)
	ClearBox bool   `json:"clearBox"` // drop a box art edit
	VMU      string `json:"vmu"`      // data URL of an image to convert
	VMUArt   bool   `json:"vmuArt"`   // make the VMU screen from the box art
	ClearVMU bool   `json:"clearVmu"`
}

func SaveGame(req SaveGameRequest) error {
	if !folderRe.MatchString(req.Folder) {
		return errors.New("bad folder")
	}
	dir := filepath.Join(req.Root, req.Folder)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	c, err := ScanCard(req.Root)
	if err != nil {
		return err
	}
	g := findGame(c, req.Folder)
	if g == nil {
		return errors.New("game folder not found")
	}
	// name.txt / serial.txt (GDMENUCardManager compatible)
	name := strings.TrimSpace(asciiOnly(req.Name))
	userName := g.UserName
	if name != "" && name != g.Name {
		userName = true
		if err := os.WriteFile(filepath.Join(dir, "name.txt"), []byte(name), 0o644); err != nil {
			return err
		}
	}
	serial := strings.TrimSpace(asciiOnly(req.Serial))
	product := g.Product
	if serial != "" && openMenuProduct(serial) != g.Product {
		if err := os.WriteFile(filepath.Join(dir, "serial.txt"), []byte(serial), 0o644); err != nil {
			return err
		}
		product = openMenuProduct(serial)
	}
	edits := loadEdits(req.Root)
	vga := req.VGA
	m := req.Meta
	edits.Games[req.Folder] = &GameEdit{Product: product, Region: strings.ToUpper(strings.TrimSpace(req.Region)), VGA: &vga, Date: strings.TrimSpace(req.Date), Meta: &m, UserName: userName}
	if err := edits.save(); err != nil {
		return err
	}
	// art
	var boxImg image.Image
	switch {
	case req.ClearBox:
		os.Remove(artPath(req.Root, req.Folder, "box"))
	case req.Box != "":
		if boxImg, err = decodeDataURL(req.Box); err != nil {
			return errors.New("could not read that image file")
		}
	case req.BoxDisc:
		b, err := readDiscFile(dir, "0GDTEX.PVR", 4<<20)
		if err != nil {
			return errors.New("this disc has no artwork file")
		}
		if boxImg, err = decodePVR(b); err != nil {
			return err
		}
	}
	if boxImg != nil {
		sq := fitSquare(boxImg, 256)
		if err := os.WriteFile(artPath(req.Root, req.Folder, "box"), pngBytes(sq), 0o644); err != nil {
			return err
		}
	}
	switch {
	case req.ClearVMU:
		os.Remove(artPath(req.Root, req.Folder, "vmu"))
	case req.VMU != "":
		img, err := decodeDataURL(req.VMU)
		if err != nil {
			return errors.New("could not read that image file")
		}
		os.WriteFile(artPath(req.Root, req.Folder, "vmu"), makeVMU(img, false), 0o644)
	case req.VMUArt:
		src := boxImg
		if src == nil {
			if b, err := GameArt(req.Root, req.Folder, "box"); err == nil {
				src, _, _ = image.Decode(bytes.NewReader(b))
			}
		}
		if src == nil {
			return errors.New("add box art first, then make the VMU screen from it")
		}
		os.WriteFile(artPath(req.Root, req.Folder, "vmu"), makeVMU(src, req.BoxDisc), 0o644)
	}
	return nil
}

// FillFromDiscs gives every game without box art its disc's own artwork (0GDTEX.PVR) and a VMU screen.
func FillFromDiscs(root string, log Logger) (int, error) {
	c, err := ScanCard(root)
	if err != nil {
		return 0, err
	}
	os.MkdirAll(filepath.Join(root, editsDir, "art"), 0o755)
	n := 0
	for _, g := range c.Games {
		if g.HasArt && g.HasVMU {
			continue
		}
		b, err := readDiscFile(filepath.Join(root, g.Folder), "0GDTEX.PVR", 4<<20)
		if err != nil {
			continue
		}
		img, err := decodePVR(b)
		if err != nil {
			continue
		}
		if !g.HasArt {
			os.WriteFile(artPath(root, g.Folder, "box"), pngBytes(fitSquare(img, 256)), 0o644)
		}
		if !g.HasVMU {
			os.WriteFile(artPath(root, g.Folder, "vmu"), makeVMU(img, true), 0o644)
		}
		// remember which serial this art belongs to
		edits := loadEdits(root)
		if edits.Games[g.Folder] == nil {
			edits.Games[g.Folder] = &GameEdit{Product: g.Product}
			edits.save()
		}
		log("%s: used the disc artwork", g.Name)
		n++
	}
	return n, nil
}

// pruneMenuBackups keeps the newest few automatic menu backups so the card does not fill up.
func pruneMenuBackups(root string, keep int) {
	var auto []string
	for _, b := range listBackups(root) { // newest first
		if strings.HasPrefix(b, "01_2") {
			auto = append(auto, b)
		}
	}
	for i := keep; i < len(auto); i++ {
		os.RemoveAll(filepath.Join(root, backupDir, auto[i]))
	}
}

// addExtras puts the owner's collections, screenshots and menu music on the menu disc.
func addExtras(root string, c *Card, data string, log Logger) error {
	removeCI := func(name string) {
		for _, e := range listDir(data) {
			if strings.EqualFold(e, name) {
				os.Remove(filepath.Join(data, e))
			}
		}
	}
	removeCI("COLLECT.TXT")
	removeCI("LOGO.VMU")
	if bits, custom := GetLogo(root); custom {
		if err := os.WriteFile(filepath.Join(data, "LOGO.VMU"), bits, 0o644); err != nil {
			return err
		}
		log("Added your VMU logo")
	}
	if cols := LoadCollections(root); len(cols) > 0 {
		if err := os.WriteFile(filepath.Join(data, "COLLECT.TXT"), []byte(collectText(cols)), 0o644); err != nil {
			return err
		}
		log("Added %d collections of your own", len(cols))
	}
	if err := buildShotDat(root, c, data, log); err != nil {
		return err
	}
	// menu music: the owner's own, else the SWIRL theme. Music another tool put on the old menu disc is
	// kept as the owner's own the first time.
	if old := findDataFile(data, "BGM.ADP"); old != "" && !fileExists(musicPath(root)) && !fileExists(filepath.Join(root, editsDir, "BGM.NONE")) && !fileExists(themeMarker(root)) {
		if b, err := os.ReadFile(old); err == nil && len(b) > 32 && string(b[:4]) == "OMBG" && !isDefaultMusic(b) {
			os.MkdirAll(filepath.Join(root, editsDir), 0o755)
			if os.WriteFile(musicPath(root), b, 0o644) == nil {
				log("Kept the menu music that was on the card as your own")
			}
		}
	}
	os.Remove(filepath.Join(root, editsDir, "BGM.NONE"))
	removeCI("BGM.ADP")
	if fileExists(musicPath(root)) {
		if err := copyFile(musicPath(root), filepath.Join(data, "BGM.ADP")); err != nil {
			return err
		}
		log("Added your menu music")
		return nil
	}
	theme, err := defaultMusic()
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(data, "BGM.ADP"), theme, 0o644); err != nil {
		return err
	}
	log("Added the SWIRL theme music (%s)", defaultMusicName)
	return nil
}

var swirlReleaseRe = regexp.MustCompile(`Version (\d+\.\d+(?:\.\d+)?)\x00`)

// swirlRelease reads the version a SWIRL menu binary shows in System > About SWIRL.
func swirlRelease(b []byte) string {
	if m := swirlReleaseRe.FindSubmatch(b); m != nil {
		return string(m[1])
	}
	return ""
}
