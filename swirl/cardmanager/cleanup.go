package main

// Duplicate detection and removal, plus folder renumbering so GDEMU sees 02, 03, 04 ... with no gaps.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type DupGroup struct {
	Name   string   `json:"name"`
	Keep   string   `json:"keep"`
	Extras []string `json:"extras"`
	Bytes  int64    `json:"bytes"` // space the extras use
}

func folderSize(dir string) (int64, []string) {
	var total int64
	var sig []string
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if isJunk(e.Name()) { // macOS "._" files and the like are not discs
			continue
		}
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext == ".txt" || ext == ".jpg" || ext == ".png" {
			continue // GDMENUCardManager's info files differ in dates only
		}
		if info, err := e.Info(); err == nil {
			total += info.Size()
			sig = append(sig, fmt.Sprintf("%s:%d", strings.ToLower(e.Name()), info.Size()))
		}
	}
	sort.Strings(sig)
	return total, sig
}

// findDuplicates groups game folders that hold the same disc: same serial, disc number, version and
// date, and the same image files at the same sizes.
func findDuplicates(root string, games []Game) []DupGroup {
	type key struct{ id, sig string }
	seen := map[key]*DupGroup{}
	var order []key
	for _, g := range games {
		if g.Product == "" || g.Error != "" {
			continue
		}
		size, sig := folderSize(filepath.Join(root, g.Folder))
		k := key{g.Product + "|" + g.Disc + "|" + g.Version + "|" + g.Date, strings.Join(sig, ",")}
		if d, ok := seen[k]; ok {
			d.Extras = append(d.Extras, g.Folder)
			d.Bytes += size
			continue
		}
		seen[k] = &DupGroup{Name: g.Name, Keep: g.Folder}
		order = append(order, k)
	}
	var out []DupGroup
	for _, k := range order {
		if d := seen[k]; len(d.Extras) > 0 {
			out = append(out, *d)
		}
	}
	return out
}

func numberedFolders(root string) []int {
	entries, _ := os.ReadDir(root)
	var nums []int
	for _, e := range entries {
		if e.IsDir() && folderRe.MatchString(e.Name()) {
			if n, _ := strconv.Atoi(e.Name()); n >= 2 {
				nums = append(nums, n)
			}
		}
	}
	sort.Ints(nums)
	return nums
}

// folderName is the folder on the card that holds game n, whichever way its number is written (02, 2,
// 002, 0002); a folder that does not exist yet is named the GDEMU way (02, 100, 1000).
func folderName(root string, n int) string {
	for _, cand := range []string{fmt.Sprintf("%02d", n), strconv.Itoa(n), fmt.Sprintf("%03d", n), fmt.Sprintf("%04d", n)} {
		if st, err := os.Stat(filepath.Join(root, cand)); err == nil && st.IsDir() {
			return cand
		}
	}
	return fmt.Sprintf("%02d", n)
}

// renumber closes gaps: the game folders become 02, 03, 04 ... in their current order. SWIRL edits and
// art follow their game. The renames are journaled (renumberTxn), so a failure part way is undone.
func renumber(root string, log Logger) (int, error) {
	var order []string
	for _, n := range numberedFolders(root) {
		order = append(order, folderName(root, n))
	}
	moved, err := moveFolders(root, order, log)
	if err == nil && moved > 0 {
		log("Renumbered %d folders so there are no gaps", moved)
	}
	return moved, err
}

// moveFolders renames the game folders so order[i] becomes 02+i. SWIRL edits and art follow their game.
func moveFolders(root string, order []string, log Logger) (int, error) {
	return renumberTxn(root, nil, "", order)
}

// renumberTxn moves the folders in remove to dest (a folder under SWIRL_BACKUP) and renames the rest so
// order[i] becomes 02+i. Every rename is journaled in SWIRL/renumber.json: on any failure the done ones
// are undone and the card is as it was. SWIRL edits and art follow their game; the art of removed games
// is deleted once everything else is done. It returns how many folders were renumbered.
func renumberTxn(root string, remove []string, dest string, order []string) (int, error) {
	edits := loadEdits(root)
	rel := func(p string) string { r, _ := filepath.Rel(root, p); return filepath.ToSlash(r) }
	j := &renumberJournal{Version: 1}
	if len(remove) > 0 {
		j.Made = rel(dest)
	}
	var dropArt []string
	for _, f := range remove {
		j.Steps = append(j.Steps, renameStep{From: f, To: j.Made + "/" + f})
		for _, kind := range []string{"box", "vmu"} {
			if p := artPath(root, f, kind); fileExists(p) {
				j.Steps = append(j.Steps, renameStep{From: rel(p), To: rel(p) + ".rm"})
				dropArt = append(dropArt, p+".rm")
			}
		}
	}
	type mv struct{ from, to string }
	var plan []mv
	for i, from := range order {
		if to := fmt.Sprintf("%02d", i+2); from != to {
			plan = append(plan, mv{from, to})
		}
	}
	// two steps so a rename never lands on a folder that has not moved yet; art the same way
	for _, p := range plan {
		j.Steps = append(j.Steps, renameStep{From: p.from, To: "_swirl_mv_" + p.from})
	}
	var artDone []renameStep
	for _, p := range plan {
		j.Steps = append(j.Steps, renameStep{From: "_swirl_mv_" + p.from, To: p.to})
		for _, kind := range []string{"box", "vmu"} {
			if src := artPath(root, p.from, kind); fileExists(src) {
				j.Steps = append(j.Steps, renameStep{From: rel(src), To: rel(artPath(root, p.to, kind)) + ".mv"})
				artDone = append(artDone, renameStep{From: rel(artPath(root, p.to, kind)) + ".mv", To: rel(artPath(root, p.to, kind))})
			}
		}
	}
	j.Steps = append(j.Steps, artDone...)
	if len(j.Steps) == 0 {
		return 0, nil
	}
	err := runRenames(root, j, func() error {
		newGames := map[string]*GameEdit{}
		for k, v := range edits.Games {
			newGames[k] = v
		}
		for _, f := range remove {
			delete(newGames, f)
		}
		for _, p := range plan {
			delete(newGames, p.from)
		}
		for _, p := range plan {
			if e := edits.Games[p.from]; e != nil {
				newGames[p.to] = e
			}
		}
		if len(newGames) == 0 && len(edits.Games) == 0 {
			return nil
		}
		edits.Games = newGames
		return edits.save()
	})
	if err != nil {
		return 0, err
	}
	for _, p := range dropArt {
		cardfs.Remove(p)
	}
	return len(plan), nil
}

// RemoveDuplicates moves the extra copies to SWIRL_BACKUP/removed_<time> (instant, same drive), closes the
// folder number gaps and rebuilds the menu. The copies can then be deleted for good from the Backups list.
func RemoveDuplicates(root string, log Logger) error {
	c, err := ScanCard(root)
	if err != nil {
		return err
	}
	dups := findDuplicates(root, c.Games)
	if len(dups) == 0 {
		return errors.New("no duplicate games found")
	}
	dest := uniqueBackupPath(root, "removed_")
	var extras []string
	gone := map[string]bool{}
	for _, d := range dups {
		for _, f := range d.Extras {
			extras = append(extras, f)
			gone[f] = true
		}
	}
	var order []string
	for _, n := range numberedFolders(root) {
		if f := folderName(root, n); !gone[f] {
			order = append(order, f)
		}
	}
	// the moves and the renumbering are one journaled step; the menu is rebuilt only when it completed
	moved, err := renumberTxn(root, extras, dest, order)
	if err != nil {
		return err
	}
	for _, d := range dups {
		for _, f := range d.Extras {
			log("Removed extra copy of %s (folder %s)", d.Name, f)
		}
	}
	if moved > 0 {
		log("Renumbered %d folders so there are no gaps", moved)
	}
	return InstallSwirl(root, "", log)
}

// DeleteRemoved permanently deletes a SWIRL_BACKUP/removed_* folder.
func DeleteRemoved(root, name string) error {
	if !strings.HasPrefix(name, "removed_") || strings.ContainsAny(name, `/\`) {
		return errors.New("only removed game copies can be deleted here")
	}
	return os.RemoveAll(filepath.Join(root, backupDir, name))
}
