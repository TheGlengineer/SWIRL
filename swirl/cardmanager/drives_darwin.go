//go:build darwin

package main

// Drives on macOS: every volume under /Volumes, described by diskutil.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// diskutilInfo runs `diskutil info -plist <what>` (a mount point, disk4 or /dev/disk4).
func diskutilInfo(what string) (plistDict, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/sbin/diskutil", "info", "-plist", what).Output()
	if err != nil {
		return nil, err
	}
	v, err := parsePlist(bytes.NewReader(out))
	if err != nil {
		return nil, err
	}
	m, _ := v.(map[string]any)
	return plistDict(m), nil
}

var (
	volCacheMu sync.Mutex
	volCache   = map[string]volCacheEntry{}
)

type volCacheEntry struct {
	at time.Time
	d  plistDict
}

// volumePlist caches diskutil's answer for a few seconds; the file browser asks often.
func volumePlist(mount string) plistDict {
	volCacheMu.Lock()
	e, ok := volCache[mount]
	volCacheMu.Unlock()
	if ok && time.Since(e.at) < 10*time.Second {
		return e.d
	}
	d, err := diskutilInfo(mount)
	if err != nil {
		d = plistDict{}
	}
	volCacheMu.Lock()
	volCache[mount] = volCacheEntry{time.Now(), d}
	volCacheMu.Unlock()
	return d
}

func fsLabel(d plistDict) string {
	n := d.str("FilesystemName") + " " + d.str("FilesystemUserVisibleName")
	switch {
	case strings.Contains(n, "FAT32"):
		return "FAT32"
	case strings.Contains(strings.ToLower(n), "exfat"):
		return "exFAT"
	case strings.Contains(n, "FAT16"):
		return "FAT16"
	case strings.Contains(n, "FAT12"):
		return "FAT12"
	case strings.Contains(n, "APFS"):
		return "APFS"
	case strings.Contains(n, "HFS") || strings.Contains(n, "Mac OS Extended"):
		return "Mac OS Extended"
	case strings.Contains(n, "NTFS"):
		return "NTFS"
	}
	return strings.TrimSpace(d.str("FilesystemType"))
}

func isRemovableVolume(d plistDict) bool {
	if b, _ := d.boolean("OSInternalMedia"); b {
		return false
	}
	r, _ := d.boolean("RemovableMedia")
	r2, _ := d.boolean("Removable")
	e, _ := d.boolean("Ejectable")
	internal, _ := d.boolean("Internal")
	return r || r2 || e || strings.EqualFold(d.str("BusProtocol"), "Secure Digital") || !internal
}

// listDrives on macOS: the startup disk and every mounted volume.
func listDrives() []Drive {
	out := []Drive{statDrive("/", "This Mac", "fixed")}
	entries, _ := os.ReadDir("/Volumes")
	for _, e := range entries {
		m := filepath.Join("/Volumes", e.Name())
		st, err := os.Lstat(m)
		if err != nil || st.Mode()&os.ModeSymlink != 0 || !st.IsDir() || !isMount(m) {
			continue // the startup disk also appears here as a link to /
		}
		d := volumePlist(m)
		if b, _ := d.boolean("OSInternalMedia"); b {
			continue
		}
		dr := statDrive(m, e.Name(), "fixed")
		if name := d.str("VolumeName"); name != "" {
			dr.Label = name
		}
		dr.FS = fsLabel(d)
		if isRemovableVolume(d) {
			dr.Type, dr.Removable = "removable", true
		}
		out = append(out, dr)
	}
	return out
}

func openBrowser(url string) error { return exec.Command("/usr/bin/open", url).Start() }

// volumeInfo reports the file system, cluster size and space of the volume holding root.
func volumeInfo(root string) (fs string, cluster int, total, free uint64) {
	d := volumePlist(root)
	fs = fsLabel(d)
	var st syscall.Statfs_t
	if syscall.Statfs(root, &st) == nil {
		cluster = int(st.Bsize)
		total = st.Blocks * uint64(st.Bsize)
		free = st.Bavail * uint64(st.Bsize)
		if fs == "" {
			fs = strings.TrimRight(string(bytes.TrimRight(int8sToBytes(st.Fstypename[:]), "\x00")), "\x00")
			if fs == "msdos" {
				fs = "FAT32"
			}
		}
	}
	if n := d.num("VolumeAllocationBlockSize"); n > 0 {
		cluster = int(n)
	}
	return
}

func int8sToBytes(a []int8) []byte {
	b := make([]byte, len(a))
	for i, v := range a {
		b[i] = byte(v)
	}
	return b
}
