//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type Drive struct {
	Path      string `json:"path"`
	Label     string `json:"label"`
	Type      string `json:"type"`
	Removable bool   `json:"removable"`
	HasMenu   bool   `json:"hasMenu"`
	Ready     bool   `json:"ready"`
	FS        string `json:"fs,omitempty"`
	Total     uint64 `json:"total,omitempty"`
	Free      uint64 `json:"free,omitempty"`
}

func isMount(p string) bool {
	var a, b syscall.Stat_t
	if syscall.Stat(p, &a) != nil || syscall.Stat(filepath.Dir(p), &b) != nil {
		return false
	}
	return a.Dev != b.Dev
}

func statDrive(p, label, typ string) Drive {
	d := Drive{Path: p, Label: label, Type: typ, Ready: true}
	var fs syscall.Statfs_t
	if syscall.Statfs(p, &fs) == nil {
		d.Total = fs.Blocks * uint64(fs.Bsize)
		d.Free = fs.Bavail * uint64(fs.Bsize)
	}
	if st, err := os.Stat(filepath.Join(p, "01")); err == nil && st.IsDir() {
		d.HasMenu = true
	}
	return d
}

func places() []Place {
	home, _ := os.UserHomeDir()
	var out []Place
	for _, n := range []string{"Desktop", "Downloads", "Documents"} {
		if st, err := os.Stat(filepath.Join(home, n)); err == nil && st.IsDir() {
			out = append(out, Place{Name: n, Path: filepath.Join(home, n)})
		}
	}
	if home != "" {
		out = append(out, Place{Name: "Home", Path: home})
	}
	return out
}

func hiddenFile(info os.FileInfo) bool { return strings.HasPrefix(info.Name(), ".") }

func freeSpace(root string) (uint64, bool) {
	var fs syscall.Statfs_t
	if syscall.Statfs(root, &fs) != nil {
		return 0, false
	}
	return fs.Bavail * uint64(fs.Bsize), true
}
