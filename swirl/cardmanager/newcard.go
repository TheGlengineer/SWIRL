package main

// New card in one shot: format the SD card to FAT32, optionally copy games, then install SWIRL.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// DiskInfo describes the physical card behind a drive letter, for the confirmation screen.
type DiskInfo struct {
	Root      string   `json:"root"`
	Disk      int      `json:"disk"`
	Model     string   `json:"model"`
	Bus       string   `json:"bus"`
	SizeBytes uint64   `json:"sizeBytes"`
	Removable bool     `json:"removable"`
	Letters   []string `json:"letters"` // every drive letter on this card
	OK        bool     `json:"ok"`
	Reason    string   `json:"reason,omitempty"`
}

type NewCardRequest struct {
	Root    string   `json:"root"`
	Disk    int      `json:"disk"`
	Confirm string   `json:"confirm"`
	Source  string   `json:"source"`
	Picks   []string `json:"picks"` // games picked one by one (folders, disc images, archives)
	Dats    string   `json:"dats"`
	Online  bool     `json:"online"`
}

type jobState struct {
	Running bool     `json:"running"`
	Done    bool     `json:"done"`
	Stage   string   `json:"stage"`
	Pct     float64  `json:"pct"`
	Log     []string `json:"log"`
	Error   string   `json:"error,omitempty"`
	Root    string   `json:"root,omitempty"`
	// the task has a Cancel button (backups)
	Cancellable bool `json:"cancellable,omitempty"`
}

var (
	jobMu sync.Mutex
	job   jobState
)

func jobUpdate(f func(j *jobState)) {
	jobMu.Lock()
	f(&job)
	jobMu.Unlock()
}

func jobSnapshot() jobState {
	jobMu.Lock()
	defer jobMu.Unlock()
	j := job
	j.Log = append([]string(nil), job.Log...)
	return j
}

func jobLog(format string, args ...any) {
	jobUpdate(func(j *jobState) { j.Log = append(j.Log, fmt.Sprintf(format, args...)) })
}

func driveLetter(root string) string {
	r := strings.TrimSpace(root)
	if len(r) >= 2 && r[1] == ':' {
		return strings.ToUpper(r[:1])
	}
	return ""
}

// copyItem is one file to copy onto the new card.
type copyItem struct {
	Src, Dst string // Dst is relative to the card root
	Entry    string // set when Src is an archive: the file inside it
	Size     int64
}

// postCheck is a game unpacked from an archive whose serial could only be read after unpacking.
type postCheck struct{ Folder, Label string }

type copyPlan struct {
	Items     []copyItem
	Checks    []postCheck
	PickBytes int64 // new card: size of the games picked one by one
	Skipped   []string
	Total     int64
	Games     int
	HasMenu   bool
	HasINI    bool
	Warnings  []string
}

var imageExts = map[string]bool{".gdi": true, ".cdi": true, ".mds": true, ".ccd": true}

func hasImage(dir string) bool {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && imageExts[strings.ToLower(filepath.Ext(e.Name()))] {
			return true
		}
	}
	return false
}

func addTree(p *copyPlan, src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		if info.Size() >= 1<<32 {
			return fmt.Errorf("%s is larger than 4 GB, which FAT32 cannot store", path)
		}
		p.Items = append(p.Items, copyItem{Src: path, Dst: filepath.Join(dst, rel), Size: info.Size()})
		p.Total += info.Size()
		return nil
	})
}

// planCopy works out what to copy from a source folder. The source can be an old GDEMU card
// (01, 02, 03 ...), a backup of one, or a folder of game folders with any names.
func planCopy(src string) (*copyPlan, error) {
	p := &copyPlan{}
	if src == "" {
		return p, nil
	}
	st, err := os.Stat(src)
	if err != nil || !st.IsDir() {
		return nil, fmt.Errorf("the games folder %s was not found", src)
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return nil, err
	}
	var numbered []int
	var named []string
	var loose []string
	maxNum := 1
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			switch {
			case strings.EqualFold(name, backupDir), strings.EqualFold(name, "System Volume Information"), strings.HasPrefix(name, "$"), strings.HasPrefix(name, "."):
				continue
			case strings.EqualFold(name, editsDir):
				if err := addTree(p, filepath.Join(src, name), editsDir); err != nil {
					return nil, err
				}
				continue
			case name == "01" || name == "1":
				if findGDI(filepath.Join(src, name)) != "" {
					p.HasMenu = true
					if err := addTree(p, filepath.Join(src, name), "01"); err != nil {
						return nil, err
					}
				}
				continue
			}
			if !hasImage(filepath.Join(src, name)) {
				continue
			}
			if folderRe.MatchString(name) {
				n, _ := strconv.Atoi(name)
				if n >= 2 {
					numbered = append(numbered, n)
					if n > maxNum {
						maxNum = n
					}
					continue
				}
			}
			named = append(named, name)
			continue
		}
		if strings.EqualFold(name, "GDEMU.INI") {
			p.HasINI = true
			info, _ := e.Info()
			p.Items = append(p.Items, copyItem{Src: filepath.Join(src, name), Dst: "GDEMU.INI", Size: info.Size()})
			continue
		}
		if strings.EqualFold(filepath.Ext(name), ".cdi") {
			loose = append(loose, name)
		}
	}
	sort.Ints(numbered)
	for _, n := range numbered {
		name := fmt.Sprintf("%02d", n)
		if _, err := os.Stat(filepath.Join(src, name)); err != nil {
			name = strconv.Itoa(n)
		}
		if err := addTree(p, filepath.Join(src, name), fmt.Sprintf("%02d", n)); err != nil {
			return nil, err
		}
		p.Games++
	}
	sort.Slice(named, func(i, j int) bool { return strings.ToLower(named[i]) < strings.ToLower(named[j]) })
	next := maxNum + 1
	for _, name := range named {
		dst := fmt.Sprintf("%02d", next)
		if err := addTree(p, filepath.Join(src, name), dst); err != nil {
			return nil, err
		}
		if readText(filepath.Join(src, name, "name.txt")) == "" {
			p.Items = append(p.Items, copyItem{Src: "text:" + name, Dst: filepath.Join(dst, "name.txt")})
		}
		next++
		p.Games++
	}
	sort.Slice(loose, func(i, j int) bool { return strings.ToLower(loose[i]) < strings.ToLower(loose[j]) })
	for _, name := range loose {
		dst := fmt.Sprintf("%02d", next)
		info, err := os.Stat(filepath.Join(src, name))
		if err != nil {
			continue
		}
		p.Items = append(p.Items, copyItem{Src: filepath.Join(src, name), Dst: filepath.Join(dst, "disc.cdi"), Size: info.Size()})
		p.Items = append(p.Items, copyItem{Src: "text:" + strings.TrimSuffix(name, filepath.Ext(name)), Dst: filepath.Join(dst, "name.txt")})
		p.Total += info.Size()
		next++
		p.Games++
	}
	if next > 999 {
		return nil, errors.New("GDEMU supports up to 999 folders")
	}
	return p, nil
}

func copyWithProgress(src, dst string, onBytes func(int64)) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	return copyStream(in, dst, onBytes)
}

func copyStream(in io.Reader, dst string, onBytes func(int64)) error {
	os.MkdirAll(filepath.Dir(dst), 0o755)
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	buf := make([]byte, 4<<20)
	for {
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				out.Close()
				return err
			}
			onBytes(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return rerr
		}
	}
	return out.Close()
}

func runCopy(p *copyPlan, root string, onPct func(float64)) error {
	var done int64
	progress := func(n int64) {
		done += n
		if p.Total > 0 {
			onPct(float64(done) / float64(p.Total))
		}
	}
	var arcs []string
	var names []copyItem
	byArc := map[string]map[string]copyItem{}
	for i, it := range p.Items {
		dst := filepath.Join(root, it.Dst)
		if it.Entry != "" {
			if byArc[it.Src] == nil {
				byArc[it.Src] = map[string]copyItem{}
				arcs = append(arcs, it.Src)
			}
			byArc[it.Src][it.Entry] = it
			continue
		}
		if strings.HasPrefix(it.Src, "text:") {
			names = append(names, it) // named once the disc is on the card and its header can be read
			continue
		}
		if i%50 == 0 || it.Size > 50<<20 {
			jobUpdate(func(j *jobState) { j.Stage = "Copying " + filepath.ToSlash(it.Dst) })
		}
		if err := copyWithProgress(it.Src, dst, progress); err != nil {
			return fmt.Errorf("copying %s: %w", it.Src, err)
		}
	}
	// each archive is read once, front to back, which is the fast way through solid 7z and RAR files
	for _, a := range arcs {
		want := byArc[a]
		got := 0
		jobUpdate(func(j *jobState) { j.Stage = "Unpacking " + filepath.Base(a) })
		err := walkArchive(a, func(n string) bool { _, ok := want[n]; return ok }, func(n string, _ int64, r io.Reader) error {
			it := want[n]
			got++
			if err := copyStream(r, filepath.Join(root, it.Dst), progress); err != nil {
				return fmt.Errorf("%s: %w", path.Base(n), err)
			}
			if got == len(want) {
				return errStopWalk
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("unpacking %s: %w", filepath.Base(a), err)
		}
		if got < len(want) {
			return fmt.Errorf("unpacking %s: %d files were missing from the archive", filepath.Base(a), len(want)-got)
		}
	}
	// proper names: the real title from the disc's serial, else a tidied file name
	for _, it := range names {
		dst := filepath.Join(root, it.Dst)
		label := strings.TrimPrefix(it.Src, "text:")
		ip, _, _ := readImageIP(filepath.Dir(dst))
		name := properName(ip, label)
		os.MkdirAll(filepath.Dir(dst), 0o755)
		if err := os.WriteFile(dst, []byte(asciiOnly(name)), 0o644); err != nil {
			return err
		}
		if name != label {
			jobLog("Named %s: %s", filepath.Base(filepath.Dir(dst)), name)
		}
	}
	return nil
}

// checkNewCard validates a request before anything is erased.
func checkNewCard(req NewCardRequest) (*DiskInfo, *copyPlan, error) {
	letter := driveLetter(req.Root)
	if letter == "" {
		return nil, nil, errors.New("pick the card by its drive letter, for example G:")
	}
	if !strings.EqualFold(strings.TrimSuffix(strings.TrimSpace(req.Confirm), ":"), letter) {
		return nil, nil, fmt.Errorf("type %s to confirm that the card in %s: should be erased", letter, letter)
	}
	info, err := diskInfo(req.Root)
	if err != nil {
		return nil, nil, err
	}
	if !info.OK {
		return nil, nil, errors.New(info.Reason)
	}
	if info.Disk != req.Disk {
		return nil, nil, errors.New("the drive changed since you picked it; pick the card again")
	}
	if l := driveLetter(req.Source); l != "" {
		for _, x := range info.Letters {
			if strings.EqualFold(driveLetter(x), l) {
				return nil, nil, errors.New("the games folder is on the card being erased; copy the games to your PC first")
			}
		}
	}
	plan, err := planCopy(strings.TrimSpace(req.Source))
	if err != nil {
		return nil, nil, err
	}
	// games picked one by one: checked and sized now, copied after formatting
	if len(req.Picks) > 0 {
		for _, pk := range req.Picks {
			if l := driveLetter(pk); l != "" {
				for _, x := range info.Letters {
					if strings.EqualFold(driveLetter(x), l) {
						return nil, nil, fmt.Errorf("%s is on the card being erased; copy it to your PC first", filepath.Base(pk))
					}
				}
			}
		}
		tmp, err := os.MkdirTemp("", "swirl-newcard")
		if err != nil {
			return nil, nil, err
		}
		pp, err := planAddTo(tmp, req.Picks, false)
		os.RemoveAll(tmp)
		if err != nil {
			return nil, nil, err
		}
		plan.PickBytes = pp.Total
	}
	// leave room for the file system and the menu
	if uint64(plan.Total+plan.PickBytes)+256<<20 > info.SizeBytes*97/100 {
		return nil, nil, fmt.Errorf("the games need %.1f GB but the card holds %.1f GB", float64(plan.Total+plan.PickBytes)/(1<<30), float64(info.SizeBytes)/(1<<30))
	}
	return info, plan, nil
}

// formatCardHook lets the tests run a new card without a real disk.
var formatCardHook = formatCard

func runNewCard(req NewCardRequest, info *DiskInfo, plan *copyPlan) {
	fail := func(err error) {
		jobUpdate(func(j *jobState) {
			j.Running, j.Done, j.Error = false, true, err.Error()
			j.Log = append(j.Log, "Stopped: "+err.Error())
		})
	}
	jobLog("Erasing and formatting %s (%s, %.1f GB) as FAT32", info.Root, info.Model, float64(info.SizeBytes)/1e9)
	jobUpdate(func(j *jobState) { j.Stage = "Formatting" })
	root, err := formatCardHook(info.Root, info.Disk, func(pct float64, msg string) {
		jobUpdate(func(j *jobState) {
			j.Pct = pct * 0.10
			if msg != "" {
				j.Stage = msg
			}
		})
	})
	if err != nil {
		fail(err)
		return
	}
	jobLog("Formatted. The card is now %s (label SWIRL)", root)
	jobUpdate(func(j *jobState) { j.Root, j.Pct = root, 0.10 })

	// progress: 10% formatting, 80% copying (shared by size between the folder copy and the picked games)
	all := float64(plan.Total + plan.PickBytes)
	share := 1.0
	if all > 0 {
		share = float64(plan.Total) / all
	}
	if plan.Games > 0 || plan.HasMenu {
		jobLog("Copying %d games (%.1f GB)", plan.Games, float64(plan.Total)/(1<<30))
		err := runCopy(plan, root, func(f float64) { jobUpdate(func(j *jobState) { j.Pct = 0.10 + 0.80*share*f }) })
		if err != nil {
			fail(err)
			return
		}
		renumber(root, jobLog)
	}
	if len(req.Picks) > 0 {
		before := cardGameKeys(root)
		pp, err := planAdd(root, req.Picks)
		if err != nil {
			fail(err)
			return
		}
		jobLog("Adding %d games you picked (%.1f GB)", pp.Games, float64(pp.Total)/(1<<30))
		for _, sk := range pp.Skipped {
			jobLog("%s", sk)
		}
		if err := runCopy(pp, root, func(f float64) { jobUpdate(func(j *jobState) { j.Pct = 0.10 + 0.80*(share+(1-share)*f) }) }); err != nil {
			fail(err)
			return
		}
		if _, err := afterUnpack(root, pp, before, jobLog); err != nil {
			fail(err)
			return
		}
		forgetCardKeys()
		rememberFolders("games", req.Picks)
	}
	if plan.Games > 0 || plan.HasMenu || len(req.Picks) > 0 {
		if _, err := keepDiscsTogether(root, jobLog); err != nil {
			fail(err)
			return
		}
	}
	if !plan.HasINI {
		if err := os.WriteFile(filepath.Join(root, "GDEMU.INI"), []byte(gdemuINI), 0o644); err != nil {
			fail(err)
			return
		}
		jobLog("Wrote GDEMU.INI (reset button returns to SWIRL)")
	} else {
		jobLog("Kept GDEMU.INI from the games folder")
	}
	if req.Online {
		if _, ok := readDBInfo(); !ok {
			jobUpdate(func(j *jobState) { j.Stage = "Downloading box art and info" })
			if err := DownloadDB(jobLog, func(f float64) { jobUpdate(func(j *jobState) { j.Pct = 0.90 + 0.02*f }) }); err != nil {
				jobLog("Online art skipped: %v", err)
			}
		}
	}
	jobUpdate(func(j *jobState) { j.Stage, j.Pct = "Installing SWIRL", 0.92 })
	if err := installSwirl(root, req.Dats, true, jobLog); err != nil {
		fail(err)
		return
	}
	jobUpdate(func(j *jobState) {
		j.Running, j.Done, j.Pct, j.Stage = false, true, 1, "Done"
		j.Log = append(j.Log, "Your card is ready. Eject it safely, then put it in the GDEMU.")
	})
}

func startNewCard(req NewCardRequest) error {
	jobMu.Lock()
	if job.Running {
		jobMu.Unlock()
		return errors.New("a card is already being prepared")
	}
	jobMu.Unlock()
	info, plan, err := checkNewCard(req)
	if err != nil {
		return err
	}
	jobUpdate(func(j *jobState) { *j = jobState{Running: true, Stage: "Starting"} })
	if plan.Games > 0 {
		jobLog("Found %d games to copy from %s", plan.Games, req.Source)
	}
	go runNewCard(req, info, plan)
	return nil
}
