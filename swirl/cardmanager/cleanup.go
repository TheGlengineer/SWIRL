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
	"time"
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

func folderName(root string, n int) string {
	for _, cand := range []string{fmt.Sprintf("%02d", n), strconv.Itoa(n), fmt.Sprintf("%03d", n)} {
		if st, err := os.Stat(filepath.Join(root, cand)); err == nil && st.IsDir() {
			return cand
		}
	}
	return fmt.Sprintf("%02d", n)
}

// renumber closes gaps: the game folders become 02, 03, 04 ... in their current order. SWIRL edits and
// art follow their game.
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
	edits := loadEdits(root)
	moved := 0
	type mv struct{ from, to string }
	var plan []mv
	for i, from := range order {
		to := fmt.Sprintf("%02d", i+2)
		if from != to {
			plan = append(plan, mv{from, to})
		}
	}
	if len(plan) == 0 {
		return 0, nil
	}
	// two steps so a rename never lands on a folder that has not moved yet
	for _, p := range plan {
		if err := os.Rename(filepath.Join(root, p.from), filepath.Join(root, "_swirl_mv_"+p.from)); err != nil {
			return moved, fmt.Errorf("renaming folder %s: %w", p.from, err)
		}
	}
	newGames := map[string]*GameEdit{}
	for k, v := range edits.Games {
		newGames[k] = v
	}
	for _, p := range plan {
		delete(newGames, p.from)
	}
	for _, p := range plan {
		if err := os.Rename(filepath.Join(root, "_swirl_mv_"+p.from), filepath.Join(root, p.to)); err != nil {
			return moved, fmt.Errorf("renaming folder %s to %s: %w", p.from, p.to, err)
		}
		if e := edits.Games[p.from]; e != nil {
			newGames[p.to] = e
		}
		for _, kind := range []string{"box", "vmu"} {
			if fileExists(artPath(root, p.from, kind)) {
				os.Rename(artPath(root, p.from, kind), artPath(root, p.to, kind)+".mv")
			}
		}
		moved++
	}
	// finish art renames (two steps for the same reason as the folders)
	matches, _ := filepath.Glob(filepath.Join(root, editsDir, "art", "*.mv"))
	for _, m := range matches {
		os.Rename(m, strings.TrimSuffix(m, ".mv"))
	}
	edits.Games = newGames
	if len(edits.Games) > 0 {
		edits.save()
	}
	return moved, nil
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
	dest := filepath.Join(root, backupDir, "removed_"+time.Now().Format("20060102_150405"))
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	edits := loadEdits(root)
	for _, d := range dups {
		for _, f := range d.Extras {
			if err := os.Rename(filepath.Join(root, f), filepath.Join(dest, f)); err != nil {
				return fmt.Errorf("moving folder %s: %w", f, err)
			}
			delete(edits.Games, f)
			os.Remove(artPath(root, f, "box"))
			os.Remove(artPath(root, f, "vmu"))
			log("Removed extra copy of %s (folder %s)", d.Name, f)
		}
	}
	edits.save()
	if _, err := renumber(root, log); err != nil {
		return err
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
