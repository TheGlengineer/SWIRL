package main

// Deciding, from `diskutil info -plist` output, whether a Mac disk may be formatted as a GDEMU card.
// Kept outside the darwin build so the rules are tested on every platform.

import (
	"fmt"
	"strconv"
	"strings"
)

// macDiskNumber turns "disk4", "disk4s1" or "/dev/disk4" into 4.
func macDiskNumber(id string) int {
	id = strings.TrimPrefix(strings.TrimPrefix(id, "/dev/"), "r")
	if !strings.HasPrefix(id, "disk") {
		return -1
	}
	s := id[4:]
	if i := strings.IndexByte(s, 's'); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return -1
	}
	return n
}

// judgeMacDisk fills in a DiskInfo for the whole disk described by whole. protected lists the disk
// numbers that hold the running macOS (the boot container and its physical store).
func judgeMacDisk(info *DiskInfo, whole plistDict, protected map[int]bool) {
	info.Disk = macDiskNumber(whole.str("DeviceIdentifier"))
	info.Model = strings.TrimSpace(whole.str("MediaName"))
	if info.Model == "" {
		info.Model = "Unknown card"
	}
	info.Bus = whole.str("BusProtocol")
	info.SizeBytes = uint64(whole.num("TotalSize"))
	if info.SizeBytes == 0 {
		info.SizeBytes = uint64(whole.num("Size"))
	}
	removable, _ := whole.boolean("RemovableMedia")
	if r, ok := whole.boolean("Removable"); ok && r {
		removable = true
	}
	ejectable, _ := whole.boolean("Ejectable")
	internal, _ := whole.boolean("Internal")
	osInternal, _ := whole.boolean("OSInternalMedia")
	info.Removable = removable || ejectable
	sdBus := strings.EqualFold(info.Bus, "Secure Digital")
	block := whole.num("DeviceBlockSize")
	switch {
	case info.Disk < 0:
		info.Reason = "this is not a disk that can be formatted"
	case protected[info.Disk] || osInternal:
		info.Reason = "this disk holds macOS and can never be formatted here"
	case strings.EqualFold(whole.str("VirtualOrPhysical"), "Virtual"):
		info.Reason = "this is a disk image or virtual disk, not an SD card"
	case block != 0 && block != secSize:
		info.Reason = fmt.Sprintf("this card uses %d byte sectors; only 512 is supported", block)
	case internal && !sdBus && !removable:
		info.Reason = fmt.Sprintf("this is an internal %s drive, not an SD card", strings.TrimSpace(info.Bus+" "+"disk"))
	case !(sdBus || removable || ejectable || strings.EqualFold(info.Bus, "USB")):
		info.Reason = "this does not look like an SD card or USB card reader"
	case !writableMedia(whole):
		info.Reason = "the card is write protected; take it out, slide its lock switch up, away from LOCK, and put it back"
	case info.SizeBytes == 0:
		info.Reason = "could not read the card size; is a card in the reader?"
	case info.SizeBytes > 2<<40:
		info.Reason = "cards larger than 2 TB are not supported"
	case info.SizeBytes < 128<<20:
		info.Reason = "the card is too small"
	default:
		info.OK = true
	}
}

// writableMedia is macOS's own reading of the card's lock switch (missing means writable).
func writableMedia(whole plistDict) bool {
	if w, ok := whole.boolean("WritableMedia"); ok {
		return w
	}
	return true
}

// confirmWord is what the person types to confirm erasing a Mac card: its name in capitals, or ERASE.
func macConfirmWord(volumeName string) string {
	w := strings.ToUpper(strings.TrimSpace(volumeName))
	if w == "" || len(w) > 24 {
		return "ERASE"
	}
	return w
}
