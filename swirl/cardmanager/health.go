package main

// Card health check: the card's format, space, folder numbering, and whether every game image is complete.

import (
	"fmt"
	"os"
	"path/filepath"

	"strings"
	"time"
)

type HealthItem struct {
	Level   string `json:"level"` // ok, warn, error
	Folder  string `json:"folder,omitempty"`
	Message string `json:"message"`
}

type HealthReport struct {
	FileSystem  string       `json:"fileSystem"`
	ClusterKB   int          `json:"clusterKB"`
	TotalBytes  uint64       `json:"totalBytes"`
	FreeBytes   uint64       `json:"freeBytes"`
	GameBytes   uint64       `json:"gameBytes"`
	Games       int          `json:"games"`
	Items       []HealthItem `json:"items"`
	JunkFiles   []string     `json:"junkFiles"`
	CheckedTime string       `json:"checked"`
}

func isJunk(name string) bool {
	return strings.HasPrefix(name, "._") || name == ".DS_Store" || name == "Thumbs.db" || name == "desktop.ini"
}

func checkGDI(path string) []string {
	var probs []string
	tracks, err := parseGDI(path)
	if err != nil {
		return []string{"the .gdi file cannot be read"}
	}
	for i, t := range tracks {
		st, err := os.Stat(t.File)
		if err != nil {
			probs = append(probs, fmt.Sprintf("track %d (%s) is missing", t.Num, filepath.Base(t.File)))
			continue
		}
		if t.SectorSize <= 0 || st.Size()%int64(t.SectorSize) != 0 {
			probs = append(probs, fmt.Sprintf("track %d is not a whole number of sectors, so the copy is probably incomplete", t.Num))
		}
		if st.Size() == 0 {
			probs = append(probs, fmt.Sprintf("track %d is empty", t.Num))
		}
		if i+1 < len(tracks) && tracks[i+1].LBA > 0 && t.LBA+t.Sectors > tracks[i+1].LBA+2 && tracks[i+1].LBA != 45000 {
			probs = append(probs, fmt.Sprintf("track %d is longer than the space the .gdi gives it", t.Num))
		}
	}
	if len(tracks) >= 3 && tracks[2].LBA != 45000 {
		probs = append(probs, "track 3 does not start at 45000, so this is not a normal GD-ROM image")
	}
	// the last data track must hold the game; a tiny one means a failed copy
	last := tracks[len(tracks)-1]
	if last.Type == 4 && last.Sectors > 0 && last.Sectors < 1000 {
		probs = append(probs, "the last data track is only a few sectors long")
	}
	return probs
}

// CheckCard runs every check. It only reads, except that it lists the junk files a Mac or Windows left.
func CheckCard(root string) (*HealthReport, error) {
	c, err := ScanCard(root)
	if err != nil {
		return nil, err
	}
	r := &HealthReport{Games: len(c.Games), CheckedTime: time.Now().Format("Jan 2 3:04 PM")}
	add := func(level, folder, msg string, args ...any) {
		r.Items = append(r.Items, HealthItem{Level: level, Folder: folder, Message: fmt.Sprintf(msg, args...)})
	}
	fs, cluster, total, free := volumeInfo(root)
	r.FileSystem, r.ClusterKB, r.TotalBytes, r.FreeBytes = fs, cluster/1024, total, free
	switch {
	case fs == "":
	case fs == "FAT32" && cluster >= 32768:
		add("ok", "", "Formatted FAT32 with %d KB clusters, as GDEMU likes", cluster/1024)
	case fs == "FAT32":
		add("warn", "", "FAT32 with %d KB clusters. GDEMU loads faster with 32 KB clusters (New card from scratch formats that way)", cluster/1024)
	case fs == "exFAT":
		add("warn", "", "The card is exFAT. Newer GDEMU firmware reads it, but many clone boards only read FAT32")
	default:
		add("error", "", "The card is %s. GDEMU needs FAT32", fs)
	}
	if total > 0 && free < total/50 {
		add("warn", "", "The card is almost full (%.1f GB free)", float64(free)/(1<<30))
	}

	// menu
	switch c.MenuType {
	case "SWIRL":
		add("ok", "01", "SWIRL is installed")
	case "None":
		add("error", "01", "There is no menu in folder 01, so GDEMU will boot the first game instead")
	case "Game":
		add("error", "01", "Folder 01 holds a game, not a menu")
	default:
		add("warn", "01", "Folder 01 has %s, not SWIRL", c.MenuType)
	}

	// folders
	nums := numberedFolders(root)
	for i, n := range nums {
		if n != i+2 {
			add("warn", folderName(root, n), "Folder numbers skip before this one; GDEMU stops at the first gap")
			break
		}
	}
	if len(nums) > 998 {
		add("error", "", "GDEMU supports up to 999 folders")
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if e.IsDir() && !folderRe.MatchString(e.Name()) && hasImage(filepath.Join(root, e.Name())) {
			add("warn", e.Name(), "This folder has a game but no number, so GDEMU will not see it (Add games can number it)")
		}
		if !e.IsDir() && isJunk(e.Name()) {
			r.JunkFiles = append(r.JunkFiles, e.Name())
		}
	}

	bad := 0
	for _, g := range c.Games {
		dir := filepath.Join(root, g.Folder)
		files, _ := os.ReadDir(dir)
		var images []string
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			name := f.Name()
			if isJunk(name) {
				r.JunkFiles = append(r.JunkFiles, filepath.Join(g.Folder, name))
				continue
			}
			if info, err := f.Info(); err == nil {
				r.GameBytes += uint64(info.Size())
			}
			if imageExts[strings.ToLower(filepath.Ext(name))] {
				images = append(images, name)
			}
		}
		var probs []string
		switch {
		case len(images) == 0:
			probs = append(probs, "no disc image (.gdi, .cdi, .mds or .ccd) in this folder")
		case len(images) > 1:
			probs = append(probs, "more than one disc image in this folder ("+strings.Join(images, ", ")+"); GDEMU may load the wrong one")
		}
		for _, im := range images {
			p := filepath.Join(dir, im)
			switch strings.ToLower(filepath.Ext(im)) {
			case ".gdi":
				probs = append(probs, checkGDI(p)...)
			case ".cdi":
				if st, err := os.Stat(p); err == nil && st.Size() < 1<<20 {
					probs = append(probs, "the .cdi file is under 1 MB")
				}
			}
		}
		if g.Error != "" && g.Type != "psx" && len(images) > 0 {
			probs = append(probs, "the game header cannot be read ("+g.Error+")")
		}
		for _, p := range probs {
			add("error", g.Folder, "%s: %s", g.Name, p)
		}
		if len(probs) > 0 {
			bad++
		}
	}
	if bad == 0 && len(c.Games) > 0 {
		add("ok", "", "All %d game images look complete", len(c.Games))
	}
	if len(c.Dups) > 0 {
		add("warn", "", "%d games are on the card more than once (see Remove extra copies)", len(c.Dups))
	}
	if len(r.JunkFiles) > 0 {
		add("warn", "", "%d leftover system files (like ._ files from a Mac). They can confuse GDEMU", len(r.JunkFiles))
	}
	return r, nil
}

// RemoveJunk moves the leftover system files into SWIRL_BACKUP/junk_<time>.
func RemoveJunk(root string) (int, error) {
	r, err := CheckCard(root)
	if err != nil {
		return 0, err
	}
	dest := filepath.Join(root, backupDir, "removed_junk_"+time.Now().Format("20060102_150405"))
	n := 0
	for _, f := range r.JunkFiles {
		to := filepath.Join(dest, strings.ReplaceAll(f, string(os.PathSeparator), "_"))
		os.MkdirAll(dest, 0o755)
		if err := os.Rename(filepath.Join(root, f), to); err == nil {
			n++
		}
	}
	return n, nil
}
