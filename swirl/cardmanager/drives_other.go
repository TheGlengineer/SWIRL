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

// listDrives on Linux and macOS: the root and anything mounted under the usual removable media folders.
func listDrives() []Drive {
	out := []Drive{statDrive("/", "Computer", "fixed")}
	for _, base := range []string{"/media", "/mnt", "/Volumes", "/run/media"} {
		matches, _ := filepath.Glob(filepath.Join(base, "*"))
		more, _ := filepath.Glob(filepath.Join(base, "*", "*"))
		for _, m := range append(matches, more...) {
			if st, err := os.Stat(m); err == nil && st.IsDir() && isMount(m) {
				d := statDrive(m, filepath.Base(m), "removable")
				d.Removable = true
				out = append(out, d)
			}
		}
	}
	return out
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

func openBrowser(url string) error { return nil }

func freeSpace(root string) (uint64, bool) {
	var fs syscall.Statfs_t
	if syscall.Statfs(root, &fs) != nil {
		return 0, false
	}
	return fs.Bavail * uint64(fs.Bsize), true
}

func volumeInfo(root string) (fs string, cluster int, total, free uint64) { return "", 0, 0, 0 }
