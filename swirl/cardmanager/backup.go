package main

// Backing up the whole SD card to a folder on the PC: every game folder, the menu in 01, SWIRL's own
// edits and GDEMU.INI. A backup has the same layout as the card, so New card from scratch can put it
// back onto a card ("copy everything from a folder").
//
// Each backup goes into its own dated folder, or an earlier backup can be brought up to date, which only
// copies what changed (a card holds hundreds of gigabytes, so that saves a lot of time).

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const backupMarker = "SWIRL-BACKUP.json"

type BackupRequest struct {
	Root       string `json:"root"`
	Dest       string `json:"dest"`       // folder on the PC that holds the backups
	IncludeOld bool   `json:"includeOld"` // also SWIRL_BACKUP (old menus, removed games)
	Update     bool   `json:"update"`     // bring the newest earlier backup up to date
}

type backupInfo struct {
	Created  string `json:"created"`
	Updated  string `json:"updated"`
	Card     string `json:"card"`
	CardID   string `json:"cardId"` // from SWIRL/card-id.txt, so updates never mix two cards
	Games    int    `json:"games"`
	Bytes    int64  `json:"bytes"`
	Files    int    `json:"files"`
	Complete bool   `json:"complete"`
	Version  string `json:"version"`
}

type BackupEntry struct {
	Name     string     `json:"name"`
	Path     string     `json:"path"`
	Info     backupInfo `json:"info"`
	ThisCard bool       `json:"thisCard"`
}

// jobCancel is set by the Cancel button of long tasks that can stop part way (backups).
var jobCancel atomic.Bool

var errCancelled = errors.New("cancelled")

type cardFile struct {
	rel  string
	size int64
	mod  time.Time
}

func skipOnCard(rel string, dir bool, includeOld bool) bool {
	top := strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]
	switch strings.ToLower(top) {
	case "system volume information", "$recycle.bin", ".trashes", ".spotlight-v100", ".fseventsd":
		return true
	case "swirl_backup":
		return !includeOld
	}
	return !dir && isJunk(filepath.Base(rel))
}

func listCardFiles(root string, includeOld bool) ([]cardFile, int64, error) {
	var files []cardFile
	var total int64
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil // unreadable entries are reported by the copy, not here
		}
		rel, _ := filepath.Rel(root, p)
		if rel == "." {
			return nil
		}
		if skipOnCard(rel, info.IsDir(), includeOld) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, cardFile{rel, info.Size(), info.ModTime()})
		total += info.Size()
		return nil
	})
	return files, total, err
}

func readBackupInfo(dir string) (backupInfo, bool) {
	var bi backupInfo
	b, err := os.ReadFile(filepath.Join(dir, backupMarker))
	if err != nil || json.Unmarshal(b, &bi) != nil {
		return bi, false
	}
	return bi, true
}

// ListBackups returns the SWIRL backups in a folder, newest first.
func ListBackups(dest string) []BackupEntry {
	var out []BackupEntry
	entries, _ := os.ReadDir(dest)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(dest, e.Name())
		if bi, ok := readBackupInfo(p); ok {
			out = append(out, BackupEntry{Name: e.Name(), Path: p, Info: bi})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Info.Updated > out[j].Info.Updated })
	return out
}

func sameVolume(a, b string) bool {
	la, lb := driveLetter(a), driveLetter(b)
	if la != "" || lb != "" {
		return strings.EqualFold(la, lb)
	}
	ca, _ := filepath.Abs(a)
	cb, _ := filepath.Abs(b)
	return ca == cb || strings.HasPrefix(cb+string(filepath.Separator), ca+string(filepath.Separator))
}

func StartBackup(req BackupRequest) error {
	root, dest := strings.TrimSpace(req.Root), strings.TrimSpace(req.Dest)
	if root == "" || dest == "" {
		return errors.New("pick the card and a folder on your PC to back it up to")
	}
	if !isDir(root) {
		return fmt.Errorf("the card %s was not found", root)
	}
	if sameVolume(root, dest) {
		return errors.New("pick a folder on your PC, not on the SD card")
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("cannot use %s (%v)", dest, err)
	}
	jobCancel.Store(false)
	return runJob("Backing up the card", "", func() error { return runBackup(req, root, dest) })
}

func runBackup(req BackupRequest, root, dest string) error {
	start := time.Now()
	jobUpdate(func(j *jobState) { j.Cancellable = true })
	jobLog("Reading the card")
	id := cardID(root) // made before listing, so it is part of the first backup too
	files, total, err := listCardFiles(root, req.IncludeOld)
	if err != nil {
		return fmt.Errorf("cannot read the card (%v)", err)
	}
	c, _ := ScanCard(root)
	games := 0
	if c != nil {
		games = len(c.Games)
	}
	// where the backup goes
	var target string
	info := backupInfo{Created: start.Format(time.RFC3339), Card: root, CardID: id, Games: games, Version: version}
	if req.Update {
		for _, b := range ListBackups(dest) {
			if b.Info.CardID == id {
				target = b.Path
				info.Created = b.Info.Created
				jobLog("Updating the backup in %s", target)
				break
			}
		}
		if target == "" {
			jobLog("There is no earlier backup of this card in %s, so this is a full backup", dest)
		}
	}
	if target == "" {
		name := "SWIRL card backup " + start.Format("2006-01-02 1504")
		target = filepath.Join(dest, name)
		for i := 2; isDir(target); i++ {
			target = filepath.Join(dest, fmt.Sprintf("%s (%d)", name, i))
		}
		jobLog("Backing up %d games (%s) to %s", games, sizeText(total), target)
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	// what needs copying: everything, or in an update only files that are new or changed
	var todo []cardFile
	var need int64
	keep := map[string]bool{backupMarker: true}
	for _, f := range files {
		keep[f.rel] = true
		if st, err := os.Stat(filepath.Join(target, f.rel)); err == nil && st.Size() == f.size && st.ModTime().Sub(f.mod).Abs() < 2*time.Second {
			continue
		}
		todo = append(todo, f)
		need += f.size
	}
	if free, ok := freeSpace(dest); ok && uint64(need)+64<<20 > free {
		return fmt.Errorf("the backup needs %s but %s has %s free", sizeText(need), dest, sizeText(int64(free)))
	}
	if req.Update && len(todo) < len(files) {
		jobLog("%d files are unchanged since the last backup; copying %d (%s)", len(files)-len(todo), len(todo), sizeText(need))
	}
	info.Files, info.Bytes = len(files), total
	writeInfo := func(complete bool) {
		info.Complete = complete
		info.Updated = time.Now().Format(time.RFC3339)
		b, _ := json.MarshalIndent(info, "", "  ")
		os.WriteFile(filepath.Join(target, backupMarker), b, 0o644)
	}
	writeInfo(false) // marks the folder as a SWIRL backup even if it stops part way

	var done int64
	lastStage := time.Time{}
	buf := make([]byte, 4<<20)
	for _, f := range todo {
		if jobCancel.Load() {
			return errCancelledBackup(target)
		}
		if time.Since(lastStage) > 300*time.Millisecond || f.size > 64<<20 {
			lastStage = time.Now()
			rel := filepath.ToSlash(f.rel)
			jobUpdate(func(j *jobState) { j.Stage = "Copying " + rel })
		}
		err := copyForBackup(filepath.Join(root, f.rel), filepath.Join(target, f.rel), f.mod, buf, func(n int64) {
			done += n
			if need > 0 {
				setPct(float64(done) / float64(need))
			}
		})
		if err == errCancelled {
			return errCancelledBackup(target)
		}
		if err != nil {
			return fmt.Errorf("copying %s: %v", filepath.ToSlash(f.rel), err)
		}
	}
	// an update also drops what is no longer on the card, so the backup matches it
	if req.Update {
		removed := 0
		filepath.Walk(target, func(p string, st os.FileInfo, err error) error {
			if err != nil || st.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(target, p)
			if !keep[rel] {
				if os.Remove(p) == nil {
					removed++
				}
			}
			return nil
		})
		removeEmptyDirs(target)
		if removed > 0 {
			jobLog("Removed %d files that are no longer on the card", removed)
		}
	}
	writeInfo(true)
	jobLog("Backup finished: %d games, %d files, %s, in %s.", games, len(files), sizeText(total), time.Since(start).Round(time.Second))
	jobLog("To put it back onto a card later, use New card from scratch and pick this folder: %s", target)
	return nil
}

func sizeText(b int64) string {
	if b >= 1<<30 {
		return fmt.Sprintf("%.1f GB", float64(b)/(1<<30))
	}
	return fmt.Sprintf("%d MB", (b+(1<<20)-1)>>20)
}

func errCancelledBackup(target string) error {
	return fmt.Errorf("stopped before the end, so the backup in %s is incomplete. Running the backup again with \"Update my last backup\" finishes it", target)
}

// copyForBackup copies one file through a temporary name and keeps its modified time, so the next
// update can tell it has not changed.
func copyForBackup(src, dst string, mod time.Time, buf []byte, onBytes func(int64)) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".part"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	for {
		if jobCancel.Load() {
			out.Close()
			os.Remove(tmp)
			return errCancelled
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				out.Close()
				os.Remove(tmp)
				return err
			}
			onBytes(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			os.Remove(tmp)
			return rerr
		}
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		return err
	}
	os.Chtimes(dst, mod, mod)
	return nil
}

func removeEmptyDirs(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	empty := true
	for _, e := range entries {
		if e.IsDir() && removeEmptyDirs(filepath.Join(dir, e.Name())) {
			os.Remove(filepath.Join(dir, e.Name()))
			continue
		}
		empty = false
	}
	return empty
}

// existingCardID reads the card's id without making one.
func existingCardID(root string) string {
	b, err := os.ReadFile(filepath.Join(root, editsDir, "card-id.txt"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// cardID names this card for backups. It is kept in the card's SWIRL folder (GDEMU ignores it).
func cardID(root string) string {
	p := filepath.Join(root, editsDir, "card-id.txt")
	if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) >= 8 {
		return strings.TrimSpace(string(b))
	}
	b := make([]byte, 8)
	rand.Read(b)
	id := hex.EncodeToString(b)
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte(id), 0o644)
	return id
}
