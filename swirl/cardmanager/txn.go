package main

// Replacing folder 01 safely. The new menu is written to SWIRL/.stage_01 on the card first and flushed.
// Then two folder renames swap it in: 01 goes to SWIRL_BACKUP, the stage becomes 01. Nothing in 01 changes
// before the second rename, and a failure at either rename puts the old menu back.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	stageName   = ".stage_01"    // under SWIRL/
	recoverName = "RECOVER.json" // under SWIRL/, written only when a swap could not be undone
)

func stagePath(root string) string   { return filepath.Join(root, editsDir, stageName) }
func recoverPath(root string) string { return filepath.Join(root, editsDir, recoverName) }

// stageFiles copies files (paths relative to src) into a fresh stage folder on the card and flushes them.
func stageFiles(root, src string, names []string) (string, error) {
	stage := stagePath(root)
	if err := cardfs.RemoveAll(stage); err != nil {
		return "", fmt.Errorf("clearing an old staging folder: %w", err)
	}
	if err := cardfs.MkdirAll(stage, 0o755); err != nil {
		return "", err
	}
	for _, n := range names {
		if err := copyFile(filepath.Join(src, n), filepath.Join(stage, n)); err != nil {
			return "", errors.Join(fmt.Errorf("writing the new menu to the card: %w", err), removeStage(root))
		}
	}
	if err := cardfs.SyncDir(stage); err != nil {
		return "", errors.Join(fmt.Errorf("writing the new menu to the card: %w", err), removeStage(root))
	}
	return stage, nil
}

func removeStage(root string) error {
	if err := cardfs.RemoveAll(stagePath(root)); err != nil {
		return fmt.Errorf("the staging folder %s/%s could not be removed: %w", editsDir, stageName, err)
	}
	return nil
}

// recoverInfo is what SWIRL/RECOVER.json says when a swap failed and could not be undone.
type recoverInfo struct {
	Version  int    `json:"version"`
	Time     string `json:"time"`
	Menu     string `json:"menu"`     // "01", missing now
	Previous string `json:"previous"` // the menu that was in 01, relative to the card root
	New      string `json:"new"`      // the menu that should have become 01
	Error    string `json:"error"`
}

// swapMenu makes stage the new 01. The current 01, if it holds anything, is renamed to backup first
// (backup is then required). The error says exactly where both menus are when something fails.
func swapMenu(root, stage, backup string) error {
	menuDir := filepath.Join(root, "01")
	moved := false
	if entries, err := cardfs.ReadDir(menuDir); err == nil {
		if len(entries) == 0 {
			if err := cardfs.Remove(menuDir); err != nil {
				return errors.Join(fmt.Errorf("removing the empty folder 01: %w; nothing was changed", err), removeStage(root))
			}
		} else {
			if backup == "" {
				return errors.New("internal: no backup name for the menu in 01")
			}
			if err := cardfs.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
				return errors.Join(fmt.Errorf("making %s: %w; nothing was changed", backupDir, err), removeStage(root))
			}
			if err := cardfs.Rename(menuDir, backup); err != nil {
				return errors.Join(fmt.Errorf("folder 01 could not be moved to %s (%w); nothing was changed. Close any program that has a file in 01 open and try again", backupDir, err), removeStage(root))
			}
			moved = true
		}
	}
	if err := cardfs.Rename(stage, menuDir); err != nil {
		if !moved {
			return errors.Join(fmt.Errorf("the new menu could not be moved into folder 01: %w", err), removeStage(root))
		}
		if err2 := cardfs.Rename(backup, menuDir); err2 != nil {
			rel := func(p string) string { r, _ := filepath.Rel(root, p); return filepath.ToSlash(r) }
			info := recoverInfo{Version: 1, Time: time.Now().Format(time.RFC3339), Menu: "01", Previous: rel(backup), New: rel(stage),
				Error: err.Error() + "; putting the old menu back: " + err2.Error()}
			b, _ := json.MarshalIndent(info, "", "  ")
			werr := writeCardFile(recoverPath(root), b)
			return errors.Join(fmt.Errorf("folder 01 could not be replaced (%v) and the old menu could not be put back (%v). "+
				"The old menu is in %s and the new one in %s; open the card again to put the old one back", err, err2, info.Previous, info.New), werr)
		}
		return errors.Join(fmt.Errorf("the new menu could not be moved into folder 01 (%w); the old menu was put back", err), removeStage(root))
	}
	cardfs.SyncDir(root)
	return nil
}

// writeCardFile writes a small file on the card and flushes it.
func writeCardFile(p string, b []byte) error {
	if err := cardfs.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := cardfs.Create(p)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := cardfs.SyncFile(f); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// menuBackupPrefix names the backup of the menu in 01. The first backup of a menu SWIRL did not build is
// kept for good as 01_original_<menu>_<time>; the rest are 01_<time> and only the newest three stay.
func menuBackupPrefix(root string, c *Card) string {
	if c == nil || (c.MenuType != "openMenu" && c.MenuType != "GDMENU") {
		return "01_"
	}
	for _, b := range listBackups(root) {
		if strings.HasPrefix(b, "01_original_") {
			return "01_"
		}
	}
	return "01_original_" + strings.ToLower(c.MenuType) + "_"
}

// checkMenuBackup refuses a backup whose .gdi is missing or names a file that is not in the backup.
func checkMenuBackup(dir string) error {
	gdi := findGDI(dir)
	if gdi == "" {
		return errors.New("that backup holds no menu disc (.gdi); nothing was changed")
	}
	tracks, err := parseGDI(gdi)
	if err != nil {
		return fmt.Errorf("that backup's .gdi cannot be read (%v); nothing was changed", err)
	}
	for _, t := range tracks {
		if filepath.Dir(t.File) != filepath.Clean(dir) || !fileExists(t.File) {
			return fmt.Errorf("that backup is incomplete: %s is missing; nothing was changed", filepath.Base(t.File))
		}
	}
	return nil
}

// recoverMenu runs at every scan while the card is not being written. It puts back the old menu when a
// swap could not be undone at the time, and removes a staging folder an interrupted update left behind.
func recoverMenu(root string) []string {
	var warn []string
	menuDir := filepath.Join(root, "01")
	has01 := fileExists(menuDir)
	if b, err := os.ReadFile(recoverPath(root)); err == nil {
		var info recoverInfo
		json.Unmarshal(b, &info)
		prev := filepath.Join(root, filepath.FromSlash(info.Previous))
		switch {
		case !has01 && info.Previous != "" && fileExists(prev):
			if err := cardfs.Rename(prev, menuDir); err != nil {
				return append(warn, fmt.Sprintf("Folder 01 is missing after an interrupted menu update. The old menu is in %s; it could not be put back (%v). Restore it from Backups.", info.Previous, err))
			}
			has01 = true
			cardfs.Remove(recoverPath(root))
			warn = append(warn, "An interrupted menu update was undone: the previous menu is back in folder 01.")
		case has01:
			cardfs.Remove(recoverPath(root))
			warn = append(warn, "An interrupted menu update was found; folder 01 is in place again.")
		default:
			return append(warn, "Folder 01 is missing after an interrupted menu update (see SWIRL/RECOVER.json). Restore the newest menu from Backups.")
		}
	}
	if fileExists(stagePath(root)) {
		if !has01 {
			return append(warn, "Folder 01 is missing after an interrupted menu update. The old menu is in Backups; restore the newest one.")
		}
		if err := removeStage(root); err != nil {
			warn = append(warn, err.Error())
		} else {
			warn = append(warn, "Removed the staging folder an interrupted menu update left behind (SWIRL/.stage_01). Folder 01 was not changed.")
		}
	}
	return warn
}
