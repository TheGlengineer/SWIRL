//go:build linux

package main

// Parsing /proc/self/mountinfo to find the mount that holds a path and what
// filesystem it is. Kept as pure functions (string in, entries out) so the
// Linux CI runner can test them without real hardware.

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// mountEntry is one line of /proc/self/mountinfo.
type mountEntry struct {
	Major, Minor int
	MountPoint   string // decoded (spaces and the like unescaped)
	FSType       string
	Source       string
}

// unescapeMountPoint decodes the octal escapes proc uses in the mount point field.
func unescapeMountPoint(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(
		`\040`, " ",
		`\011`, "\t",
		`\012`, "\n",
		`\134`, `\`,
	).Replace(s)
}

func parseMajorMinor(s string) (int, int, error) {
	maj, min, ok := strings.Cut(s, ":")
	if !ok {
		return 0, 0, strconv.ErrSyntax
	}
	m, err := strconv.Atoi(maj)
	if err != nil {
		return 0, 0, err
	}
	n, err := strconv.Atoi(min)
	if err != nil {
		return 0, 0, err
	}
	return m, n, nil
}

// parseMountinfo parses /proc/self/mountinfo text into mount entries.
//
// Line layout (fields split on spaces, see proc(5)):
//
//	36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue
//
// field 0 mount ID, 1 parent ID, 2 major:minor, 3 root, 4 mount point,
// 5 mount options, then optional fields until a lone "-", then the filesystem
// type, source and super options.
func parseMountinfo(text string) []mountEntry {
	var out []mountEntry
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		f := strings.Split(line, " ")
		if len(f) < 7 {
			continue
		}
		maj, min, err := parseMajorMinor(f[2])
		if err != nil {
			continue
		}
		sep := -1
		for i := 6; i < len(f); i++ {
			if f[i] == "-" {
				sep = i
				break
			}
		}
		if sep < 0 || sep+1 >= len(f) {
			continue
		}
		e := mountEntry{
			Major:      maj,
			Minor:      min,
			MountPoint: unescapeMountPoint(f[4]),
			FSType:     f[sep+1],
		}
		if sep+2 < len(f) {
			e.Source = unescapeMountPoint(f[sep+2])
		}
		out = append(out, e)
	}
	return out
}

// readMountinfo reads and parses the running system's /proc/self/mountinfo.
func readMountinfo() ([]mountEntry, error) {
	b, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	return parseMountinfo(string(b)), nil
}

// mountEntryFor returns the mount whose mount point is the longest prefix of root
// ("/" is the fallback for everything, so a path that is not itself a mount point
// still resolves to the filesystem that holds it).
func mountEntryFor(entries []mountEntry, root string) (mountEntry, bool) {
	root = filepath.Clean(root)
	best := -1
	var out mountEntry
	found := false
	for _, e := range entries {
		mp := filepath.Clean(e.MountPoint)
		switch {
		case mp == "/":
			if best < 0 {
				out, best, found = e, 0, true
			}
		case root == mp || strings.HasPrefix(root, mp+string(filepath.Separator)):
			if len(mp) > best {
				out, best, found = e, len(mp), true
			}
		}
	}
	return out, found
}

// fsLabelLinux maps a Linux filesystem type to the name SWIRL shows. GDEMU cards
// are FAT32, and statfs cannot tell FAT12/16/32 apart, so the vfat family is
// reported as FAT32 (the same simplification macOS makes).
func fsLabelLinux(fstype string) string {
	switch strings.ToLower(strings.TrimSpace(fstype)) {
	case "vfat", "msdos", "fat":
		return "FAT32"
	case "exfat":
		return "exFAT"
	case "ntfs":
		return "NTFS"
	case "ext2", "ext3", "ext4":
		return "ext4"
	case "":
		return ""
	}
	return strings.TrimSpace(fstype)
}
