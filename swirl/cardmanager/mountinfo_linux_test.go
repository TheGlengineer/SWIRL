//go:build linux

package main

import "testing"

const sampleMountinfo = `36 35 98:0 /mnt1 /mnt2 rw,noatime master:1 - ext3 /dev/root rw,errors=continue
37 24 8:17 / / rw,relatime shared:1 - ext4 /dev/sda1 rw,errors=remount-ro
38 35 8:33 / /media/james/SWIRL rw,nosuid,nodev shared:20 - vfat /dev/sdb1 rw
39 35 8:49 / /media/james/Space\040Card rw shared:21 - exfat /dev/sdc1 rw
40 35 8:65 / /media/james/Untitled rw shared:22 - ntfs /dev/sdd1 rw
`

func TestParseMountinfo(t *testing.T) {
	entries := parseMountinfo(sampleMountinfo)
	if len(entries) != 5 {
		t.Fatalf("parsed %d entries, want 5", len(entries))
	}
	if entries[2].FSType != "vfat" || entries[2].Source != "/dev/sdb1" {
		t.Errorf("vfat entry wrong: %+v", entries[2])
	}
	if entries[2].Major != 8 || entries[2].Minor != 33 {
		t.Errorf("major:minor wrong: %+v", entries[2])
	}
	if entries[3].MountPoint != "/media/james/Space Card" {
		t.Errorf("unescape failed: %q", entries[3].MountPoint)
	}
}

func TestMountEntryFor(t *testing.T) {
	entries := parseMountinfo(sampleMountinfo)
	cases := []struct {
		root, wantFS, wantMP string
	}{
		{"/media/james/SWIRL", "vfat", "/media/james/SWIRL"},
		{"/media/james/SWIRL/01", "vfat", "/media/james/SWIRL"},
		{"/media/james/Space Card/game", "exfat", "/media/james/Space Card"},
		{"/etc/passwd", "ext4", "/"},
		{"/", "ext4", "/"},
	}
	for _, c := range cases {
		e, ok := mountEntryFor(entries, c.root)
		if !ok {
			t.Errorf("mountEntryFor(%q): not found", c.root)
			continue
		}
		if e.MountPoint != c.wantMP || e.FSType != c.wantFS {
			t.Errorf("mountEntryFor(%q) = (%q, %q), want (%q, %q)", c.root, e.MountPoint, e.FSType, c.wantMP, c.wantFS)
		}
	}
}

func TestFsLabelLinux(t *testing.T) {
	cases := map[string]string{
		"vfat": "FAT32", "msdos": "FAT32", "fat": "FAT32",
		"exfat": "exFAT", "ntfs": "NTFS", "ext4": "ext4",
		"ext2": "ext4", "": "", "btrfs": "btrfs",
	}
	for in, want := range cases {
		if got := fsLabelLinux(in); got != want {
			t.Errorf("fsLabelLinux(%q) = %q, want %q", in, got, want)
		}
	}
}
