//go:build linux

package main

// Drives on Linux: the root and anything mounted under the usual removable media
// folders. volumeInfo fills in the filesystem type, cluster size and free/total
// space from /proc/self/mountinfo and statfs.

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
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
				d.FS = fsAt(m)
				out = append(out, d)
			}
		}
	}
	return out
}

func openBrowser(url string) error { return exec.Command("xdg-open", url).Start() }

// filesystem magic numbers from linux/magic.h.
const (
	magicMSDOS = 0x4d44
	magicExFAT = 0x2011BAB0
	magicNTFS  = 0x5346544E
	magicEXT   = 0xEF53
	magicBTRFS = 0x9123683E
)

// fsFromType maps a statfs Type field to a filesystem name. It cannot tell
// FAT12/16/32 apart (vfat reports the MSDOS magic for all of them), which is
// fine: GDEMU cards are FAT32, and that is how the rest of SWIRL treats vfat.
func fsFromType(t int64) string {
	switch uint32(t) {
	case magicMSDOS:
		return "FAT32"
	case magicExFAT:
		return "exFAT"
	case magicNTFS:
		return "NTFS"
	case magicEXT:
		return "ext4"
	case magicBTRFS:
		return "btrfs"
	}
	return ""
}

// fsAt returns the filesystem label for the mount that holds root.
func fsAt(root string) string {
	if entries, err := readMountinfo(); err == nil {
		if e, ok := mountEntryFor(entries, root); ok && e.FSType != "" {
			return fsLabelLinux(e.FSType)
		}
	}
	var st syscall.Statfs_t
	if syscall.Statfs(root, &st) != nil {
		return ""
	}
	return fsFromType(st.Type)
}

// volumeInfo reports the file system, cluster size and space of the volume holding root.
func volumeInfo(root string) (fs string, cluster int, total, free uint64) {
	var st syscall.Statfs_t
	if syscall.Statfs(root, &st) != nil {
		return "", 0, 0, 0
	}
	cluster = int(st.Bsize)
	total = st.Blocks * uint64(st.Bsize)
	free = st.Bavail * uint64(st.Bsize)
	fstype := ""
	if entries, err := readMountinfo(); err == nil {
		if e, ok := mountEntryFor(entries, root); ok && e.FSType != "" {
			fstype = e.FSType
		}
	}
	if fstype == "" {
		fs = fsFromType(st.Type)
	} else {
		fs = fsLabelLinux(fstype)
	}
	return fs, cluster, total, free
}
