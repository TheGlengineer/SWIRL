package main

// Deciding, from the Linux sysfs view of a block device, whether it may be
// formatted as a GDEMU card. Kept outside the linux build (like macdisk.go) so
// the rules are tested on every platform with fixture data.

import (
	"fmt"
	"hash/fnv"
	"path/filepath"
	"strconv"
	"strings"
)

// linuxBlockDev is the Linux sysfs view of a whole block device.
type linuxBlockDev struct {
	Name      string // "sdb", "mmcblk0", "nvme0n1"
	Model     string
	USB       bool
	Removable bool
	SizeBytes uint64
	SecSize   int
}

// parseSysfsBlockLink splits a /sys/dev/block/<maj>:<min> symlink target like
// ".../block/sdb/sdb1" or ".../block/nvme0n1/nvme0n1p1" into the whole device and
// the partition. For a whole device the link ends in .../block/sdb with no
// partition, so part is "".
func parseSysfsBlockLink(link string) (whole, part string) {
	link = strings.TrimRight(link, "/")
	base := filepath.Base(link)
	parent := filepath.Base(filepath.Dir(link))
	if parent == "block" {
		return base, ""
	}
	return parent, base
}

// linuxDevHash maps a device name to a stable int key for DiskInfo.Disk. The app
// sends the key back to confirm the drive has not changed between picking it and
// formatting it.
func linuxDevHash(name string) int {
	h := fnv.New32a()
	h.Write([]byte("/dev/" + name))
	return int(h.Sum32() & 0x7fffffff)
}

// linuxConfirmWord is what the person types to confirm erasing a card. Linux mount
// points are long and awkward to retype, so the confirmation is always the fixed word
// CONFIRM rather than a device or volume name (as Windows and macOS use).
func linuxConfirmWord(device string) string {
	return "CONFIRM"
}

// judgeLinuxDisk fills in a DiskInfo for a whole block device. protected is the
// set of physical device names that hold the running system's root filesystem.
// An empty protected set means the system disk could not be determined, so no
// disk is allowed (fail closed).
func judgeLinuxDisk(info *DiskInfo, dev linuxBlockDev, protected map[string]bool) {
	info.Disk = linuxDevHash(dev.Name)
	info.Model = strings.TrimSpace(dev.Model)
	if info.Model == "" {
		info.Model = "Unknown card"
	}
	if dev.USB {
		info.Bus = "USB"
	}
	info.SizeBytes = dev.SizeBytes
	info.Removable = dev.Removable || dev.USB
	usbOrRemovable := dev.USB || dev.Removable
	switch {
	case dev.Name == "":
		info.Reason = "this is not a disk that can be formatted"
	case len(protected) == 0:
		info.Reason = "could not find which disk holds Linux, so no disk can be formatted"
	case protected[dev.Name]:
		info.Reason = "this disk holds Linux and can never be formatted here"
	case dev.SecSize != 0 && dev.SecSize != secSize:
		info.Reason = fmt.Sprintf("this card uses %d byte sectors; only %d is supported", dev.SecSize, secSize)
	case !usbOrRemovable:
		info.Reason = fmt.Sprintf("this is an internal %s drive, not an SD card", strings.TrimSpace(info.Model))
	case dev.SizeBytes == 0:
		info.Reason = "could not read the card size; is a card in the reader?"
	case dev.SizeBytes > 2<<40:
		info.Reason = "cards larger than 2 TB are not supported"
	case dev.SizeBytes < 128<<20:
		info.Reason = "the card is too small"
	default:
		info.OK = true
	}
}

// partDev is the first partition of a whole block device name: sdb -> sdb1,
// mmcblk0 -> mmcblk0p1, nvme0n1 -> nvme0n1p1.
func partDev(whole string) string {
	if whole != "" && whole[len(whole)-1] >= '0' && whole[len(whole)-1] <= '9' {
		return whole + "p1"
	}
	return whole + "1"
}

// parseSysfsBool reads a "0"/"1" sysfs file.
func parseSysfsBool(s string) bool { return strings.TrimSpace(s) == "1" }

// parseSysfsSectors reads a sysfs size file (in 512-byte sectors) into bytes.
func parseSysfsSectors(s string) uint64 {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n * 512
}
