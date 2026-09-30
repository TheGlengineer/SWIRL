package main

// The list the old menu had. openMenu keeps OPENMENU.INI on its disc, GDMENU keeps LIST.INI; both hold
// one entry per slot with name, disc, vga, region, version and date, and openMenu adds product. Names,
// regions, VGA flags and dates the owner edited in another manager live only there, and other menus
// add keys of their own (the Virtual Folder Bundle's folder and type lines, for one). On the first
// install SWIRL reads that list: a name that differs from the disc's becomes name.txt, region, VGA and
// date differences become SWIRL edits, and every other key is kept in SWIRL/legacy_ini.json by product
// and written back into OPENMENU.INI at each rebuild, so nothing the old menu knew is lost.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const legacyININame = "legacy_ini.json"

func legacyINIPath(root string) string { return filepath.Join(root, editsDir, legacyININame) }

// iniKnownKeys are the per slot keys SWIRL writes itself; anything else is an extra to carry.
var iniKnownKeys = map[string]bool{"name": true, "disc": true, "vga": true, "region": true, "version": true, "date": true, "product": true, "type": true}

type legacyINI struct {
	Version  int                          `json:"version"`
	Menu     string                       `json:"menu"`
	Imported string                       `json:"imported"`
	Header   map[string]string            `json:"header,omitempty"` // extra keys of the [OPENMENU] or [GDMENU] section
	Items    map[string]map[string]string `json:"items,omitempty"`  // extra keys per game, by product
}

// parseMenuINI reads an openMenu or GDMENU list: the header section's keys and each slot's keys.
func parseMenuINI(text string) (header map[string]string, slots map[int]map[string]string) {
	header = map[string]string{}
	slots = map[int]map[string]string{}
	section := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\xef\xbb\xbf"))
		if line == "" || line[0] == ';' || line[0] == '#' {
			continue
		}
		if line[0] == '[' {
			section = strings.ToUpper(strings.Trim(line, "[]"))
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if slot, key, ok := strings.Cut(k, "."); ok {
			if n, err := strconv.Atoi(slot); err == nil && n > 0 {
				if slots[n] == nil {
					slots[n] = map[string]string{}
				}
				slots[n][strings.ToLower(key)] = v
				continue
			}
		}
		if section == "OPENMENU" || section == "GDMENU" || section == "" {
			header[strings.ToLower(k)] = v
		}
	}
	return header, slots
}

// readOldMenuList returns the list file of the menu in 01 (OPENMENU.INI for openMenu, LIST.INI for
// GDMENU), or "" when there is none.
func readOldMenuList(root string, c *Card) (string, string) {
	want := ""
	switch c.MenuType {
	case "openMenu":
		want = "OPENMENU.INI"
	case "GDMENU":
		want = "LIST.INI"
	default:
		return "", ""
	}
	d, err := openGameDisc(filepath.Join(root, "01"))
	if err != nil {
		return "", ""
	}
	defer d.Close()
	files, err := listISO(d)
	if err != nil {
		return "", ""
	}
	for _, f := range files {
		if f.Dir || !strings.EqualFold(f.Path, want) || f.Size > 4<<20 {
			continue
		}
		b, err := d.readSectors(f.LBA, (f.Size+sectorSize-1)/sectorSize)
		if err != nil {
			return "", ""
		}
		return string(b[:f.Size]), want
	}
	return "", ""
}

func loadLegacyINI(root string) *legacyINI {
	b, err := os.ReadFile(legacyINIPath(root))
	if err != nil {
		return nil
	}
	var l legacyINI
	if json.Unmarshal(b, &l) != nil || l.Version != 1 {
		return nil
	}
	return &l
}

// importOldMenuList runs once, when SWIRL goes onto a card whose 01 holds another menu: it keeps what
// that menu's list knew that the discs do not say.
func importOldMenuList(root string, c *Card, log Logger) (changed bool, err error) {
	if c.MenuType != "openMenu" && c.MenuType != "GDMENU" {
		return false, nil
	}
	if fileExists(legacyINIPath(root)) {
		return false, nil
	}
	text, listName := readOldMenuList(root, c)
	if text == "" {
		return false, nil
	}
	header, slots := parseMenuINI(text)
	l := &legacyINI{Version: 1, Menu: c.MenuType, Imported: time.Now().Format(time.RFC3339), Header: map[string]string{}, Items: map[string]map[string]string{}}
	for k, v := range header {
		if k != "num_items" {
			l.Header[k] = v
		}
	}
	edits := loadEdits(root)
	names, edited, extras := 0, 0, 0
	for i := range c.Games {
		g := &c.Games[i]
		s := slots[g.Slot]
		if s == nil {
			continue
		}
		if p := s["product"]; p != "" && !sameSerial(p, g.Product) {
			continue // the list is for a different disc in this slot
		}
		dir := filepath.Join(root, g.Folder)
		if name := s["name"]; name != "" && !g.Custom && name != g.Name && !fileExists(filepath.Join(dir, "name.txt")) {
			if err := os.WriteFile(filepath.Join(dir, "name.txt"), []byte(name), 0o644); err == nil {
				g.Name, g.Custom = name, true
				names++
			}
		}
		// region, VGA and date the old list had differently from the disc become an edit; a field the
		// owner already edited in SWIRL is left alone
		e := edits.Games[g.Folder]
		if e == nil || e.Product != g.Product {
			e = &GameEdit{Product: g.Product}
		}
		touched := false
		if r := strings.ToUpper(s["region"]); r != "" && r != g.Region && e.Region == "" {
			e.Region, touched = r, true
		}
		if v := s["vga"]; (v == "0" && g.VGA || v == "1" && !g.VGA) && e.VGA == nil {
			vga := v == "1"
			e.VGA, touched = &vga, true
		}
		if d := s["date"]; d != "" && d != "N/A" && d != g.Date && e.Date == "" {
			e.Date, touched = d, true
		}
		if touched {
			edits.Games[g.Folder] = e
			edited++
		}
		for k, v := range s {
			if iniKnownKeys[k] {
				continue
			}
			if l.Items[g.Product] == nil {
				l.Items[g.Product] = map[string]string{}
			}
			l.Items[g.Product][k] = v
			extras++
		}
	}
	if edited > 0 {
		if err := edits.save(); err != nil {
			return false, err
		}
	}
	os.MkdirAll(filepath.Dir(legacyINIPath(root)), 0o755)
	b, _ := json.MarshalIndent(l, "", "  ")
	if err := os.WriteFile(legacyINIPath(root), b, 0o644); err != nil {
		return false, err
	}
	log("Read the old menu's %s: %d names kept as name.txt, %d games with their region, VGA or date kept as edits, %d other settings kept in SWIRL/%s", listName, names, edited, extras, legacyININame)
	return names > 0 || edited > 0, nil
}

// legacyLines returns the extra lines for a game's entry in OPENMENU.INI, sorted for a stable file.
func (l *legacyINI) legacyLines(slot string, product string) string {
	if l == nil {
		return ""
	}
	extra := l.Items[product]
	if len(extra) == 0 {
		return ""
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s.%s=%s\r\n", slot, k, extra[k])
	}
	return b.String()
}

func (l *legacyINI) headerLines() string {
	if l == nil || len(l.Header) == 0 {
		return ""
	}
	keys := make([]string, 0, len(l.Header))
	for k := range l.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\r\n", k, l.Header[k])
	}
	return b.String()
}

// gdmenuExtras are the files a GDMENU disc carries that SWIRL can use and openMenu does not ship.
var gdmenuExtras = []string{"BLEEM.BIN"}

// carryGDMENUFiles copies BLEEM.BIN and the like from a GDMENU disc in 01 into the new menu's data.
func carryGDMENUFiles(root string, c *Card, data string, log Logger) {
	if c.MenuType != "GDMENU" {
		return
	}
	d, err := openGameDisc(filepath.Join(root, "01"))
	if err != nil {
		return
	}
	defer d.Close()
	files, err := listISO(d)
	if err != nil {
		return
	}
	for _, f := range files {
		for _, want := range gdmenuExtras {
			if f.Dir || !strings.EqualFold(f.Path, want) || findDataFile(data, want) != "" {
				continue
			}
			if err := extractISOFile(d, f, filepath.Join(data, want)); err == nil {
				log("Kept %s from the GDMENU disc", want)
			}
		}
	}
}
