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
	if moved {
		if err := cardfs.SyncDir(filepath.Dir(backup)); err != nil {
			if uerr := undoMove(root, stage, backup); uerr != nil {
				return errors.Join(fmt.Errorf("flushing %s: %w", backupDir, err), uerr)
			}
			return fmt.Errorf("flushing %s: %w; the old menu was put back", backupDir, err)
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
	if err := cardfs.SyncDir(root); err != nil {
		return fmt.Errorf("the new menu is in folder 01 but the card could not be flushed (%w); eject the card safely before removing it", err)
	}
	return nil
}

// undoMove puts the menu moved to backup back into 01 and removes the stage (before the stage was swapped in).
func undoMove(root, stage, backup string) error {
	if err := cardfs.Rename(backup, filepath.Join(root, "01")); err != nil {
		return fmt.Errorf("putting the old menu back from %s: %w", backup, err)
	}
	return removeStage(root)
}

// writeCardFile writes a small file on the card and flushes it and its folder.
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
	if err := f.Close(); err != nil {
		return err
	}
	return cardfs.SyncDir(filepath.Dir(p))
}

// checkMenuBackup refuses a backup that is incomplete: a file its manifest lists is missing or short,
// there is no disc image, or its .gdi names a file that is not in the backup.
func checkMenuBackup(dir string) error {
	if err := checkBackupComplete(dir); err != nil {
		return err
	}
	gdi := findGDI(dir)
	if gdi == "" {
		return nil // a CDI menu
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

// ---------- journaled folder renames (renumber, reorder, remove) ----------

const journalName = "renumber.json" // under SWIRL/

func journalPath(root string) string { return filepath.Join(root, editsDir, journalName) }

type renameStep struct {
	From string `json:"from"` // relative to the card root, with forward slashes
	To   string `json:"to"`
	Done bool   `json:"done"`
}

// renumberJournal is SWIRL/renumber.json: the planned renames and which are done. It exists only while a
// renumbering runs, so finding one means the last one was interrupted.
type renumberJournal struct {
	Version  int          `json:"version"`
	Steps    []renameStep `json:"steps"`
	HadEdits bool         `json:"hadEdits"`        // SWIRL/games.json existed before
	Edits    string       `json:"edits,omitempty"` // and held this
	Made     string       `json:"made,omitempty"`  // a folder made for the move, removed on undo if empty
}

func (j *renumberJournal) write(root string) error {
	b, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := journalPath(root) + ".tmp"
	if err := writeCardFile(tmp, b); err != nil {
		return err
	}
	if err := cardfs.Rename(tmp, journalPath(root)); err != nil {
		return err
	}
	return cardfs.SyncDir(filepath.Join(root, editsDir))
}

func cardPath(root, rel string) string { return filepath.Join(root, filepath.FromSlash(rel)) }

// runRenames applies the steps under a journal. finish runs once every rename is done; if a rename or
// finish fails, the done steps are undone and games.json is put back.
func runRenames(root string, j *renumberJournal, finish func() error) error {
	if b, err := os.ReadFile(filepath.Join(root, editsDir, "games.json")); err == nil {
		j.HadEdits, j.Edits = true, string(b)
	}
	if err := j.write(root); err != nil {
		cardfs.Remove(journalPath(root))
		cardfs.Remove(journalPath(root) + ".tmp")
		return fmt.Errorf("writing %s/%s: %w; nothing was changed", editsDir, journalName, err)
	}
	fail := func(err error) error {
		if uerr := undoRenames(root, j); uerr != nil {
			return errors.Join(err, fmt.Errorf("putting the folders back also failed (%w); open the card again to finish putting them back", uerr))
		}
		return fmt.Errorf("%w; the folders were put back as they were", err)
	}
	if j.Made != "" {
		if err := cardfs.MkdirAll(cardPath(root, j.Made), 0o755); err != nil {
			return fail(err)
		}
	}
	for i := range j.Steps {
		s := &j.Steps[i]
		if err := cardfs.Rename(cardPath(root, s.From), cardPath(root, s.To)); err != nil {
			return fail(fmt.Errorf("renaming %s to %s: %w", s.From, s.To, err))
		}
		s.Done = true
		if err := j.write(root); err != nil {
			return fail(fmt.Errorf("updating %s/%s: %w", editsDir, journalName, err))
		}
	}
	if err := finish(); err != nil {
		return fail(err)
	}
	// every rename must be on the card before the journal goes
	dirs := []string{root, filepath.Join(root, editsDir, "art")}
	if j.Made != "" {
		dirs = append(dirs, cardPath(root, j.Made), filepath.Dir(cardPath(root, j.Made)))
	}
	for _, d := range dirs {
		if _, err := cardfs.Stat(d); err != nil {
			continue
		}
		if err := cardfs.SyncDir(d); err != nil {
			return fail(fmt.Errorf("flushing the card: %w", err))
		}
	}
	if err := cardfs.Remove(journalPath(root)); err != nil {
		// the next scan would undo the move; undo it now so the card and its menu stay in step
		return fail(fmt.Errorf("removing %s/%s: %w", editsDir, journalName, err))
	}
	return nil
}

// undoRenames reverses the done steps of a journal, newest first, puts games.json back and removes the
// journal. A step whose rename happened but was not yet marked done is undone too.
func undoRenames(root string, j *renumberJournal) error {
	var errs []error
	exists := func(rel string) bool { _, err := cardfs.Stat(cardPath(root, rel)); return err == nil }
	last := -1
	for i, s := range j.Steps {
		if s.Done {
			last = i
		}
	}
	if n := last + 1; n < len(j.Steps) && !exists(j.Steps[n].From) && exists(j.Steps[n].To) {
		j.Steps[n].Done = true
		last = n
	}
	for i := last; i >= 0; i-- {
		s := &j.Steps[i]
		if !s.Done {
			continue
		}
		if exists(s.To) && !exists(s.From) {
			if err := cardfs.Rename(cardPath(root, s.To), cardPath(root, s.From)); err != nil {
				errs = append(errs, fmt.Errorf("renaming %s back to %s: %w", s.To, s.From, err))
				continue
			}
		}
		s.Done = false
		j.write(root) // best effort: the rename above is what matters
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	games := filepath.Join(root, editsDir, "games.json")
	if j.HadEdits {
		if err := writeCardFile(games, []byte(j.Edits)); err != nil {
			return fmt.Errorf("putting games.json back: %w", err)
		}
	} else if err := cardfs.Remove(games); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("putting games.json back: %w", err)
	}
	if j.Made != "" {
		cardfs.Remove(cardPath(root, j.Made)) // only succeeds when empty, which is the point
	}
	cardfs.Remove(journalPath(root) + ".tmp")
	if err := cardfs.Remove(journalPath(root)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// recoverRenumber runs at every scan while the card is not being written: an unfinished renumbering is
// undone, and hidden _swirl_mv_ folders with no journal are reported.
func recoverRenumber(root string) []string {
	var warn []string
	if b, err := os.ReadFile(journalPath(root)); err == nil {
		var j renumberJournal
		if err := json.Unmarshal(b, &j); err != nil || j.Version != 1 {
			warn = append(warn, fmt.Sprintf("%s/%s from an interrupted renumbering cannot be read, so it was left alone. Check the game folders before changing anything.", editsDir, journalName))
		} else if err := undoRenames(root, &j); err != nil {
			warn = append(warn, fmt.Sprintf("An interrupted renumbering of the game folders was found but could not be undone: %v", err))
		} else {
			warn = append(warn, "An interrupted renumbering of the game folders was found and undone. The folders are as they were before it.")
		}
	}
	return warn
}

// hiddenFolderWarnings reports game folders an interrupted renumbering hid from GDEMU.
func hiddenFolderWarnings(root string) []string {
	var warn []string
	for _, n := range listDir(root) {
		if strings.HasPrefix(n, "_swirl_mv_") && !fileExists(journalPath(root)) {
			warn = append(warn, fmt.Sprintf("Folder %s holds a game that an interrupted renumbering hid from GDEMU. Rename it to the next free folder number, then open the card again.", n))
		}
	}
	return warn
}

// partFolders lists the NN.part folders an add copies into.
func partFolders(root string) []string {
	var out []string
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".part") && folderRe.MatchString(strings.TrimSuffix(e.Name(), ".part")) {
			out = append(out, e.Name())
		}
	}
	return out
}

// recoverParts runs at a scan while the card is not being written: an NN.part folder is a copy that
// never finished, so it is removed and reported.
func recoverParts(root string) []string {
	var warn []string
	for _, p := range partFolders(root) {
		if err := cardfs.RemoveAll(filepath.Join(root, p)); err != nil {
			warn = append(warn, fmt.Sprintf("Folder %s is an unfinished copy of a game and could not be removed (%v). Delete it by hand.", p, err))
			continue
		}
		warn = append(warn, fmt.Sprintf("Removed folder %s, an unfinished copy of a game from an add that was interrupted. Add that game again.", p))
	}
	return warn
}
