package main

// The file browser behind Add games, Choose music and the DAT folder picker.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type Place struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type BrowseEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Dir      bool   `json:"dir"`
	Kind     string `json:"kind"` // dir, game (folder with a disc), disc, archive, music, dat, file
	Game     bool   `json:"game"` // can be picked as a game
	Size     int64  `json:"size,omitempty"`
	Modified int64  `json:"modified,omitempty"` // unix seconds
	Label    string `json:"label,omitempty"`
}

type Crumb struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type BrowseResult struct {
	Path    string        `json:"path"`
	Parent  string        `json:"parent"`
	Crumbs  []Crumb       `json:"crumbs"`
	Entries []BrowseEntry `json:"entries"`
	Drives  []Drive       `json:"drives,omitempty"`
	Places  []Place       `json:"places,omitempty"`
	Recent  []string      `json:"recent,omitempty"`
	Hidden  int           `json:"hidden"` // files not shown because they are not games, music or DATs
}

var musicExts = map[string]bool{".wav": true, ".mp3": true, ".adp": true}

func crumbs(p string) []Crumb {
	p = filepath.Clean(p)
	var out []Crumb
	for {
		parent := filepath.Dir(p)
		name := filepath.Base(p)
		if parent == p {
			out = append([]Crumb{{Name: strings.TrimRight(p, `\/`) + string(filepath.Separator), Path: p}}, out...)
			if runtime.GOOS != "windows" {
				out[0].Name = "/"
			}
			break
		}
		out = append([]Crumb{{Name: name, Path: p}}, out...)
		p = parent
	}
	return out
}

// Browse lists one folder. kind is "games" (default), "music" or "dats".
func Browse(p, kind string) (*BrowseResult, error) {
	r := &BrowseResult{Path: p, Drives: listDrives(), Places: places(), Recent: recentFolders(kind)}
	if p == "" {
		return r, nil
	}
	p = filepath.Clean(p)
	r.Path = p
	st, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("%s was not found", p)
	}
	if !st.IsDir() {
		p = filepath.Dir(p)
		r.Path = p
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("Windows does not allow opening %s", p)
		}
		return nil, fmt.Errorf("cannot open %s", p)
	}
	r.Crumbs = crumbs(p)
	if parent := filepath.Dir(p); parent != p {
		r.Parent = parent
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "$") || name == "System Volume Information" {
			continue
		}
		info, err := e.Info()
		if err != nil || hiddenFile(info) {
			continue
		}
		full := filepath.Join(p, name)
		be := BrowseEntry{Name: name, Path: full, Modified: info.ModTime().Unix()}
		if e.IsDir() || info.Mode()&os.ModeSymlink != 0 && isDir(full) {
			be.Dir = true
			be.Kind = "dir"
			if kind == "" || kind == "games" {
				if hasImage(full) {
					be.Kind, be.Game = "game", true
				}
			}
			r.Entries = append(r.Entries, be)
			continue
		}
		be.Size = info.Size()
		ext := strings.ToLower(filepath.Ext(name))
		switch kind {
		case "music":
			if musicExts[ext] {
				be.Kind, be.Game = "music", true
			}
		case "dats":
			if ext == ".dat" {
				be.Kind = "dat"
			}
		default:
			switch {
			case imageExts[ext]:
				be.Kind, be.Game = "disc", true
			case isArchivePart(name):
			case isArchive(name):
				be.Kind, be.Game = "archive", true
			}
		}
		if be.Kind == "" {
			r.Hidden++
			continue
		}
		r.Entries = append(r.Entries, be)
	}
	sort.SliceStable(r.Entries, func(i, j int) bool {
		if r.Entries[i].Dir != r.Entries[j].Dir {
			return r.Entries[i].Dir
		}
		return naturalLess(strings.ToLower(r.Entries[i].Name), strings.ToLower(r.Entries[j].Name))
	})
	return r, nil
}

func isDir(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// ---------- recently used folders ----------

var recentMu sync.Mutex

func recentPath() string { return filepath.Join(filepath.Dir(dbDir()), "recent.json") }

func loadRecent() map[string][]string {
	m := map[string][]string{}
	if b, err := os.ReadFile(recentPath()); err == nil {
		json.Unmarshal(b, &m)
	}
	return m
}

func recentFolders(kind string) []string {
	recentMu.Lock()
	defer recentMu.Unlock()
	var out []string
	for _, p := range loadRecent()[kindKey(kind)] {
		if isDir(p) {
			out = append(out, p)
		}
	}
	return out
}

func kindKey(kind string) string {
	if kind == "" {
		return "games"
	}
	return kind
}

// rememberFolders keeps the last few folders things were picked from, per kind.
func rememberFolders(kind string, picked []string) {
	recentMu.Lock()
	defer recentMu.Unlock()
	m := loadRecent()
	k := kindKey(kind)
	list := m[k]
	for _, p := range picked {
		d := p
		if !isDir(p) {
			d = filepath.Dir(p)
		} else if kind == "" || kind == "games" {
			d = filepath.Dir(p) // a picked game folder: remember the folder it sits in
		}
		var next []string
		next = append(next, d)
		for _, x := range list {
			if !strings.EqualFold(x, d) {
				next = append(next, x)
			}
		}
		list = next
	}
	if len(list) > 6 {
		list = list[:6]
	}
	m[k] = list
	os.MkdirAll(filepath.Dir(recentPath()), 0o755)
	b, _ := json.MarshalIndent(m, "", "  ")
	os.WriteFile(recentPath(), b, 0o644)
}
