package main

// The menus kept in SWIRL_BACKUP. The menu that was in 01 before SWIRL was first installed is pinned as
// 01_original_<menu>_<time>: it is listed first, never pruned, and never overwritten. Menus SWIRL built
// itself are 01_<time> and only the newest few stay.

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// BackupItem is one folder in SWIRL_BACKUP as the Backups page shows it.
type BackupItem struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`   // original, menu, replaced, removed, junk, other
	Label  string `json:"label"`  // one line for the page
	Menu   string `json:"menu"`   // the menu the folder holds: openMenu, GDMENU, SWIRL, ... ("" for removed games)
	Pinned bool   `json:"pinned"` // never pruned
	// from the manifest, when the backup has one
	When    string `json:"when,omitempty"`
	Reason  string `json:"reason,omitempty"`
	Release string `json:"release,omitempty"` // SWIRL version of a SWIRL menu
	Games   int    `json:"games,omitempty"`   // game folders on the card when it was saved
	Note    string `json:"note,omitempty"`    // why it cannot be restored as it is
}

// autoBackupRe matches the automatic backups of menus SWIRL built (01_<date>_<time>, with _2, _3 ... in
// the same second). Only these are ever pruned.
var autoBackupRe = regexp.MustCompile(`^01_\d{8}_\d{6}(_\d+)?$`)

// originalPrefix starts the pinned backup of the menu SWIRL replaced on its first install.
const originalPrefix = "01_original_"

// menuKindName is the short name used in a pinned backup's folder name.
func menuKindName(c *Card) string {
	switch {
	case c.MenuVariant != "":
		return strings.ToLower(c.MenuVariant)
	case c.MenuType == "openMenu" || c.MenuType == "GDMENU":
		return strings.ToLower(c.MenuType)
	}
	return "menu"
}

// dirSizes describes the files in a folder by size and the hash of their first 64 KB, enough to tell two
// copies of a menu apart without reading whole tracks.
func dirSizes(dir string) map[string]string {
	out := map[string]string{}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || isJunk(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		head := make([]byte, 64<<10)
		n := 0
		if f, err := os.Open(filepath.Join(dir, e.Name())); err == nil {
			n, _ = io.ReadFull(f, head)
			f.Close()
		}
		out[strings.ToLower(e.Name())] = fmt.Sprintf("%d:%x", info.Size(), sha1.Sum(head[:n]))
	}
	return out
}

func sameSizes(a, b map[string]string) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// menuBackupPrefix names the backup of the menu in 01. A menu SWIRL did not build is pinned for good as
// 01_original_<menu>_<time>, unless a pinned backup already holds that same menu (it was restored and
// SWIRL is going back in). Menus SWIRL built are 01_<time>, and only the newest three of those stay.
func menuBackupPrefix(root string, c *Card) string {
	if c == nil || c.MenuType == "SWIRL" || c.MenuType == "None" || c.MenuType == "Game" || c.MenuType == "Unknown" {
		return "01_"
	}
	cur := dirSizes(filepath.Join(root, "01"))
	for _, b := range listBackups(root) {
		if strings.HasPrefix(b, originalPrefix) && sameSizes(dirSizes(filepath.Join(root, backupDir, b)), cur) {
			return "01_"
		}
	}
	return originalPrefix + menuKindName(c) + "_"
}

// pruneMenuBackups keeps the newest few automatic menu backups so the card does not fill up. Pinned
// originals, replaced menus, removed games, anything not named by SWIRL, and a backup that holds
// something other than a menu SWIRL built (a game, an unknown menu) are left alone.
func pruneMenuBackups(root string, keep int) {
	var auto []string
	for _, b := range listBackups(root) { // newest first
		if !autoBackupRe.MatchString(b) {
			continue
		}
		if k := backupMenuKind(filepath.Join(root, backupDir, b)); k != "openMenu" && k != "" {
			continue
		}
		auto = append(auto, b)
	}
	for i := keep; i < len(auto); i++ {
		if cardfs.RemoveAll(filepath.Join(root, backupDir, auto[i])) == nil {
			cardfs.Remove(manifestPath(filepath.Join(root, backupDir, auto[i])))
		}
	}
}

// backupMenuKind names the menu a backup folder holds from its disc header ("" when there is no image).
func backupMenuKind(dir string) string {
	ip, _, err := readImageIP(dir)
	if err != nil || ip == nil {
		return ""
	}
	switch {
	case strings.Contains(ip.Name, "openMenu"):
		return "openMenu"
	case strings.Contains(strings.ToUpper(ip.Name), "GDMENU"):
		return "GDMENU"
	}
	return ip.Name
}

// listBackupItems describes every folder in SWIRL_BACKUP: the pinned original first, then the rest
// newest first.
func listBackupItems(root string) []BackupItem {
	var out []BackupItem
	for _, name := range listBackups(root) {
		it := BackupItem{Name: name}
		when := backupTime(name)
		switch {
		case strings.HasPrefix(name, originalPrefix):
			it.Kind, it.Pinned = "original", true
			it.Menu = backupMenuKind(filepath.Join(root, backupDir, name))
			menu := it.Menu
			if menu == "" {
				menu = "the old menu"
			}
			it.Label = fmt.Sprintf("The menu that was on the card before SWIRL (%s), kept for good", menu)
		case autoBackupRe.MatchString(name):
			it.Kind = "menu"
			it.Menu = backupMenuKind(filepath.Join(root, backupDir, name))
			it.Label = "Menu replaced by an update" + when
		case strings.HasPrefix(name, "01_replaced_"):
			it.Kind = "replaced"
			it.Menu = backupMenuKind(filepath.Join(root, backupDir, name))
			it.Label = "Menu replaced by a restore" + when
		case strings.HasPrefix(name, "removed_junk_"):
			it.Kind = "junk"
			it.Label = "Leftover system files" + when
		case strings.HasPrefix(name, "removed_"):
			it.Kind = "removed"
			it.Label = "Removed games" + when
		default:
			it.Kind = "other"
			it.Label = "Not made by SWIRL Card Manager"
		}
		if it.Kind == "original" || it.Kind == "menu" || it.Kind == "replaced" {
			dir := filepath.Join(root, backupDir, name)
			var parts []string
			if m := readBackupManifest(dir); m != nil { // the manifest knows a SWIRL menu from stock openMenu
				it.Reason, it.Release, it.Games = m.Reason, m.SwirlRelease, len(m.Slots)
				if t, err := time.Parse(time.RFC3339, m.Time); err == nil {
					it.When = t.Format("2006-01-02 15:04")
				}
				if m.Menu != "" {
					it.Menu = m.Menu
				}
				if it.Reason != "" {
					parts = append(parts, "saved by "+it.Reason)
				}
				if it.Release != "" {
					parts = append(parts, "SWIRL "+it.Release)
				}
				parts = append(parts, fmt.Sprintf("%d games", it.Games))
			}
			if it.Menu != "" && it.Kind != "original" {
				it.Label += " (" + it.Menu + ")"
			}
			if len(parts) > 0 {
				it.Label += ": " + strings.Join(parts, ", ")
			}
			if err := checkBackupComplete(dir); err != nil {
				it.Note = strings.TrimSuffix(err.Error(), "; nothing was changed")
			}
		}
		out = append(out, it)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pinned && !out[j].Pinned })
	return out
}

var backupTimeRe = regexp.MustCompile(`(\d{4})(\d{2})(\d{2})_(\d{2})(\d{2})(\d{2})`)

// backupTime reads the date out of a backup folder name, as ", 2026-09-29 18:41" ("" when it has none).
func backupTime(name string) string {
	m := backupTimeRe.FindStringSubmatch(name)
	if m == nil {
		return ""
	}
	return fmt.Sprintf(", %s-%s-%s %s:%s", m[1], m[2], m[3], m[4], m[5])
}

// ---------- backup manifests ----------

// manifestPath is SWIRL_BACKUP/<name>.json, next to the backup folder so the folder stays exactly what
// was in 01.
func manifestPath(dir string) string { return dir + ".json" }

type manifestSlot struct {
	Product string `json:"product"`
	Name    string `json:"name"`
	Disc    string `json:"disc,omitempty"`
}

// backupManifest is SWIRL_BACKUP/<name>.json: what the backup holds and what the card looked
// like when it was made, so a restore can tell whether the old menu still matches the game folders.
type backupManifest struct {
	Version      int                     `json:"version"`
	Time         string                  `json:"time"`
	Reason       string                  `json:"reason"`
	Menu         string                  `json:"menu"`
	MenuTitle    string                  `json:"menuTitle,omitempty"`
	MenuVariant  string                  `json:"menuVariant,omitempty"`
	MenuImage    string                  `json:"menuImage,omitempty"`
	MenuFormat   string                  `json:"menuFormat,omitempty"`
	SwirlRelease string                  `json:"swirlRelease,omitempty"`
	CardManager  string                  `json:"cardManager"`
	Files        map[string]int64        `json:"files"`
	Slots        map[string]manifestSlot `json:"slots"`
}

// writeBackupManifest describes the menu that was just moved into dir. c is the card as it was scanned
// before the move (its MenuType is the menu now in dir).
func writeBackupManifest(root, dir string, c *Card, reason string) error {
	if reason == "" {
		reason = cardOp(root)
	}
	if reason == "" {
		reason = "Install SWIRL"
	}
	m := backupManifest{Version: 1, Time: time.Now().Format(time.RFC3339), Reason: reason, CardManager: version,
		Files: map[string]int64{}, Slots: map[string]manifestSlot{}}
	if c != nil {
		m.Menu, m.MenuTitle, m.MenuVariant, m.MenuImage, m.MenuFormat, m.SwirlRelease = c.MenuType, c.MenuTitle, c.MenuVariant, c.MenuImage, c.MenuFormat, c.SwirlRelease
		for _, g := range c.Games {
			m.Slots[g.Folder] = manifestSlot{Product: g.Product, Name: g.Name, Disc: g.Disc}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() || isJunk(e.Name()) {
			continue
		}
		if info, err := e.Info(); err == nil {
			m.Files[e.Name()] = info.Size()
		}
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	return writeCardFile(manifestPath(dir), b)
}

func readBackupManifest(dir string) *backupManifest {
	b, err := os.ReadFile(manifestPath(dir))
	if err != nil {
		return nil
	}
	var m backupManifest
	if json.Unmarshal(b, &m) != nil || m.Version != 1 {
		return nil
	}
	return &m
}

// checkBackupComplete refuses a backup that lost a file since it was made (the manifest lists every file
// with its size) or whose disc image is missing.
func checkBackupComplete(dir string) error {
	if m := readBackupManifest(dir); m != nil {
		var missing []string
		for name, size := range m.Files {
			st, err := os.Stat(filepath.Join(dir, name))
			switch {
			case err != nil:
				missing = append(missing, name+" is missing")
			case st.Size() != size:
				missing = append(missing, fmt.Sprintf("%s is %d bytes, was %d", name, st.Size(), size))
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("that backup is incomplete: %s; nothing was changed", strings.Join(missing, ", "))
		}
	}
	if menuImageIn01(dir) == "" {
		return errors.New("that backup holds no menu disc; nothing was changed")
	}
	return nil
}

// restoreMismatch is the error a restore gives when the old menu's game list no longer matches the folders.
type restoreMismatch struct {
	Lines []string
}

func (e *restoreMismatch) Error() string {
	return "the game folders have changed since that menu was saved, so it would start the wrong games: " + strings.Join(e.Lines, "; ")
}

// slotMismatches compares the folders a backup's menu was built for with the folders on the card now.
func slotMismatches(m *backupManifest, c *Card) []string {
	if m == nil || m.Slots == nil || c == nil {
		return nil
	}
	now := map[string]Game{}
	for _, g := range c.Games {
		now[g.Folder] = g
	}
	var folders []string
	for f := range m.Slots {
		folders = append(folders, f)
	}
	for f := range now {
		if _, ok := m.Slots[f]; !ok {
			folders = append(folders, f)
		}
	}
	sort.Slice(folders, func(i, j int) bool { return naturalLess(folders[i], folders[j]) })
	var out []string
	for _, f := range folders {
		was, had := m.Slots[f]
		is, has := now[f]
		switch {
		case had && !has:
			out = append(out, fmt.Sprintf("slot %s was %s, is now empty", f, was.Name))
		case !had && has:
			out = append(out, fmt.Sprintf("slot %s is new (%s), the old menu does not list it", f, is.Name))
		case was.Product != is.Product || (was.Disc != "" && is.Disc != "" && was.Disc != is.Disc):
			out = append(out, fmt.Sprintf("slot %s was %s, is now %s", f, was.Name, is.Name))
		}
	}
	return out
}
