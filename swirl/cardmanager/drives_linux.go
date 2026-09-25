//go:build !windows && !darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// listDrives on Linux: the root and anything mounted under the usual removable media folders.
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

func openBrowser(url string) error { return exec.Command("xdg-open", url).Start() }

func volumeInfo(root string) (fs string, cluster int, total, free uint64) { return "", 0, 0, 0 }
