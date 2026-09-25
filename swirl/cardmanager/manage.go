package main

// Card management: add, remove and reorder games, plus a simple folder browser for the web UI.
// Every change ends with a menu rebuild so the Dreamcast sees it.

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------- background jobs (shared with New card) ----------

func runJob(stage, doneMsg string, fn func() error) error {
	jobMu.Lock()
	if job.Running {
		jobMu.Unlock()
		return errors.New("another task is still running; wait for it to finish")
	}
	job = jobState{Running: true, Stage: stage}
	jobMu.Unlock()
	go func() {
		err := fn()
		jobUpdate(func(j *jobState) {
			j.Running, j.Done = false, true
			if err != nil {
				j.Error = err.Error()
				j.Log = append(j.Log, "Stopped: "+err.Error())
				return
			}
			j.Pct, j.Stage = 1, "Done"
			if doneMsg != "" {
				j.Log = append(j.Log, doneMsg)
			}
		})
	}()
	return nil
}

func setPct(p float64) { jobUpdate(func(j *jobState) { j.Pct = p }) }

// ---------- add ----------

// gdiTracks lists the files a .gdi refers to (the .gdi itself included).
func gdiTracks(gdi string) []string {
	out := []string{gdi}
	b, err := os.ReadFile(gdi)
	if err != nil {
		return out
	}
	dir := filepath.Dir(gdi)
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		name := f[4]
		if strings.HasPrefix(name, `"`) { // quoted names can contain spaces
			if i := strings.Index(line, `"`); i >= 0 {
				if j := strings.Index(line[i+1:], `"`); j >= 0 {
					name = line[i+1 : i+1+j]
				}
			}
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out
}

// planAdd works out how to copy each picked item (a game folder, a disc file or an archive) into new card
// folders. Games that are already on the card, or picked twice, are skipped.
func planAdd(root string, sources []string) (*copyPlan, error) {
	return planAddTo(root, sources, true)
}

// planAddTo is planAdd; checkSpace is false when sizing games for a card that is not formatted yet.
func planAddTo(root string, sources []string, checkSpace bool) (*copyPlan, error) {
	p := &copyPlan{}
	nums := numberedFolders(root)
	next := 2
	if len(nums) > 0 {
		next = nums[len(nums)-1] + 1
	}
	forgetCardKeys()
	keys := map[string]string{}
	for k, v := range cardGameKeys(root) {
		keys[k] = v
	}
	// seen reports a game that is already on the card or already in this batch
	seen := func(ip *ipInfo, label, dst string) bool {
		k := ipKey(ip)
		if k == "" {
			return false
		}
		if f, ok := keys[k]; ok {
			where := "already on the card in folder " + f
			if strings.HasPrefix(f, "+") {
				where = "it is the same disc as " + f[1:]
			}
			p.Skipped = append(p.Skipped, fmt.Sprintf("Skipped %s: %s", label, where))
			return true
		}
		keys[k] = "+" + label
		return false
	}
	for _, src := range sources {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		st, err := os.Stat(src)
		if err != nil {
			return nil, fmt.Errorf("%s was not found", src)
		}
		if !st.IsDir() && isArchive(src) {
			entries, err := listArchive(src)
			if err != nil {
				return nil, err
			}
			games := gamesInArchive(src, entries)
			if len(games) == 0 {
				return nil, fmt.Errorf("%s has no Dreamcast disc (.gdi, .cdi, .mds or .ccd) inside", filepath.Base(src))
			}
			for _, g := range games {
				dst := fmt.Sprintf("%02d", next)
				ip, known := archiveIP(src, g)
				if known && seen(ip, g.Name, dst) {
					continue
				}
				for _, f := range g.Files {
					if f.Size >= 1<<32 {
						return nil, fmt.Errorf("%s in %s is larger than 4 GB, which FAT32 cannot store", path.Base(f.Name), filepath.Base(src))
					}
					p.Items = append(p.Items, copyItem{Src: src, Entry: f.Name, Dst: filepath.Join(dst, path.Base(f.Name)), Size: f.Size})
					p.Total += f.Size
				}
				p.Items = append(p.Items, copyItem{Src: "text:" + g.Name, Dst: filepath.Join(dst, "name.txt")})
				if !known {
					p.Checks = append(p.Checks, postCheck{Folder: dst, Label: g.Name})
				}
				next++
				p.Games++
			}
			continue
		}
		dst := fmt.Sprintf("%02d", next)
		label := filepath.Base(src)
		var folder string
		var items []copyItem
		var total int64
		if st.IsDir() {
			if !hasImage(src) {
				return nil, fmt.Errorf("%s has no .gdi, .cdi, .mds or .ccd disc in it", src)
			}
			sub := &copyPlan{}
			if err := addTree(sub, src, dst); err != nil {
				return nil, err
			}
			items, total, folder = sub.Items, sub.Total, src
		} else {
			ext := strings.ToLower(filepath.Ext(src))
			var files []string
			switch ext {
			case ".gdi":
				files = gdiTracks(src)
			case ".cdi":
				files = []string{src}
			case ".mds", ".ccd":
				base := strings.TrimSuffix(src, filepath.Ext(src))
				matches, _ := filepath.Glob(base + ".*")
				files = matches
			default:
				return nil, fmt.Errorf("%s is not a Dreamcast disc (.gdi, .cdi, .mds or .ccd) or an archive (.zip, .7z, .rar)", src)
			}
			for _, f := range files {
				info, err := os.Stat(f)
				if err != nil {
					return nil, fmt.Errorf("%s is missing (needed by %s)", filepath.Base(f), filepath.Base(src))
				}
				if info.Size() >= 1<<32 {
					return nil, fmt.Errorf("%s is larger than 4 GB, which FAT32 cannot store", f)
				}
				items = append(items, copyItem{Src: f, Dst: filepath.Join(dst, filepath.Base(f)), Size: info.Size()})
				total += info.Size()
			}
			label = strings.TrimSuffix(label, filepath.Ext(label))
			if ext == ".gdi" { // a .gdi is usually named disc.gdi; its folder says which game it is
				label = filepath.Base(filepath.Dir(src))
			}
			folder = filepath.Dir(src)
		}
		if ip := sourceIP(src, st, folder); ip != nil && seen(ip, label, dst) {
			continue
		}
		p.Items = append(p.Items, items...)
		p.Total += total
		if !folderRe.MatchString(label) && readText(filepath.Join(src, "name.txt")) == "" {
			p.Items = append(p.Items, copyItem{Src: "text:" + label, Dst: filepath.Join(dst, "name.txt")})
		}
		next++
		p.Games++
	}
	if p.Games == 0 {
		if len(p.Skipped) > 0 {
			return nil, errors.New("every game you picked is already on the card")
		}
		return nil, errors.New("pick at least one game")
	}
	if next-1 > 999 {
		return nil, errors.New("GDEMU supports up to 999 folders")
	}
	if free, ok := freeSpace(root); checkSpace && ok && uint64(p.Total)+64<<20 > free {
		return nil, fmt.Errorf("the games need %.1f GB but the card has %.1f GB free", float64(p.Total)/(1<<30), float64(free)/(1<<30))
	}
	return p, nil
}

// sourceIP reads the disc header of a picked folder or disc file. A loose .cdi is read on its own, because
// its folder may hold other games too.
func sourceIP(src string, st os.FileInfo, folder string) *ipInfo {
	ext := strings.ToLower(filepath.Ext(src))
	if !st.IsDir() && ext != ".gdi" {
		target := src
		if ext == ".mds" || ext == ".ccd" {
			base := strings.TrimSuffix(src, filepath.Ext(src))
			for _, e := range []string{".mdf", ".img", ".MDF", ".IMG"} {
				if fileExists(base + e) {
					target = base + e
				}
			}
		}
		ip, _ := scanForIP(target)
		return ip
	}
	ip, _, err := readImageIP(folder)
	if err != nil {
		return nil
	}
	return ip
}

// afterUnpack checks the games whose serial could only be read once they were unpacked, removes any that
// turned out to be on the card already, and closes the gaps in the folder numbers.
func afterUnpack(root string, p *copyPlan, before map[string]string, log Logger) (int, error) {
	if len(p.Checks) == 0 {
		return 0, nil
	}
	keys := map[string]string{}
	for k, v := range before {
		keys[k] = v
	}
	// keys for the new games that were checked before copying
	for _, n := range numberedFolders(root) {
		f := folderName(root, n)
		pending := false
		for _, c := range p.Checks {
			if c.Folder == f {
				pending = true
			}
		}
		if pending {
			continue
		}
		if ip, _, err := readImageIP(filepath.Join(root, f)); err == nil {
			if k := ipKey(ip); k != "" {
				if _, ok := keys[k]; !ok {
					keys[k] = f
				}
			}
		}
	}
	removed := 0
	for _, c := range p.Checks {
		dir := filepath.Join(root, c.Folder)
		ip, _, err := readImageIP(dir)
		if err != nil {
			log("Folder %s (%s): the disc header could not be read (%v)", c.Folder, c.Label, err)
			continue
		}
		k := ipKey(ip)
		if f, ok := keys[k]; ok && k != "" {
			if err := os.RemoveAll(dir); err != nil {
				return removed, err
			}
			log("Skipped %s: already on the card in folder %s", c.Label, f)
			removed++
			continue
		}
		if k != "" {
			keys[k] = c.Folder
		}
	}
	if removed > 0 {
		if _, err := renumber(root, log); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

func StartAddGames(root string, sources []string, dats string) error {
	before := cardGameKeys(root)
	p, err := planAdd(root, sources)
	if err != nil {
		return err
	}
	return runJob("Copying games", "Done. Put the card back in your GDEMU.", func() error {
		jobLog("Adding %d games (%.1f GB)", p.Games, float64(p.Total)/(1<<30))
		for _, s := range p.Skipped {
			jobLog("%s", s)
		}
		if err := runCopy(p, root, func(f float64) { setPct(0.9 * f) }); err != nil {
			return err
		}
		if n, err := afterUnpack(root, p, before, jobLog); err != nil {
			return err
		} else if n == p.Games {
			jobLog("Every game you picked was already on the card, so nothing was added.")
		}
		forgetCardKeys()
		if _, err := keepDiscsTogether(root, jobLog); err != nil {
			return err
		}
		jobUpdate(func(j *jobState) { j.Stage = "Rebuilding the menu" })
		return installSwirl(root, dats, true, jobLog)
	})
}

// ---------- remove ----------

func StartRemoveGames(root string, folders []string) error {
	nums := map[string]bool{}
	for _, n := range numberedFolders(root) {
		nums[folderName(root, n)] = true
	}
	for _, f := range folders {
		if !nums[f] {
			return fmt.Errorf("folder %s is not a game folder on this card", f)
		}
	}
	if len(folders) == 0 {
		return errors.New("pick at least one game")
	}
	return runJob("Removing games", "Done. The removed games are in SWIRL_BACKUP until you delete them.", func() error {
		dest := filepath.Join(root, backupDir, "removed_"+time.Now().Format("20060102_150405"))
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		edits := loadEdits(root)
		for _, f := range folders {
			if err := os.Rename(filepath.Join(root, f), filepath.Join(dest, f)); err != nil {
				return fmt.Errorf("moving folder %s: %w", f, err)
			}
			delete(edits.Games, f)
			os.Remove(artPath(root, f, "box"))
			os.Remove(artPath(root, f, "vmu"))
			jobLog("Removed folder %s", f)
		}
		edits.save()
		setPct(0.3)
		if _, err := renumber(root, jobLog); err != nil {
			return err
		}
		jobUpdate(func(j *jobState) { j.Stage, j.Pct = "Rebuilding the menu", 0.5 })
		return installSwirl(root, "", true, jobLog)
	})
}

// ---------- reorder ----------

// reorderFolders renames game folders so that order[i] becomes folder 02+i. SWIRL edits and art follow.
func reorderFolders(root string, order []string, log Logger) (int, error) {
	current := numberedFolders(root)
	if len(order) != len(current) {
		return 0, errors.New("the game list changed; scan the card again")
	}
	have := map[string]bool{}
	for _, n := range current {
		have[folderName(root, n)] = true
	}
	seen := map[string]bool{}
	for _, f := range order {
		if !have[f] || seen[f] {
			return 0, errors.New("the game list changed; scan the card again")
		}
		seen[f] = true
	}
	return moveFolders(root, order, log)
}

func StartReorder(root string, order []string) error {
	return runJob("Reordering games", "Done. Put the card back in your GDEMU.", func() error {
		n, err := reorderFolders(root, order, jobLog)
		if err != nil {
			return err
		}
		jobLog("Moved %d folders", n)
		jobUpdate(func(j *jobState) { j.Stage, j.Pct = "Rebuilding the menu", 0.5 })
		return installSwirl(root, "", true, jobLog)
	})
}

func naturalLess(a, b string) bool {
	na, ea := strconv.Atoi(a)
	nb, eb := strconv.Atoi(b)
	if ea == nil && eb == nil {
		return na < nb
	}
	return a < b
}
