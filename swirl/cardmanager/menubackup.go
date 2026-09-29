package main

// The menus kept in SWIRL_BACKUP. The menu that was in 01 before SWIRL was first installed is pinned as
// 01_original_<menu>_<time>: it is listed first, never pruned, and never overwritten. Menus SWIRL built
// itself are 01_<time> and only the newest few stay.

import (
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// BackupItem is one folder in SWIRL_BACKUP as the Backups page shows it.
type BackupItem struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`   // original, menu, replaced, removed, junk, other
	Label  string `json:"label"`  // one line for the page
	Menu   string `json:"menu"`   // the menu the folder holds: openMenu, GDMENU, SWIRL, ... ("" for removed games)
	Pinned bool   `json:"pinned"` // never pruned
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
		cardfs.RemoveAll(filepath.Join(root, backupDir, auto[i]))
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
		if it.Menu != "" && it.Kind != "original" {
			it.Label += " (" + it.Menu + ")"
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
