package main

import (
	"strings"
	"testing"
)

const sdReaderPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>BusProtocol</key>
	<string>USB</string>
	<key>DeviceBlockSize</key>
	<integer>512</integer>
	<key>DeviceIdentifier</key>
	<string>disk4</string>
	<key>DeviceNode</key>
	<string>/dev/disk4</string>
	<key>Ejectable</key>
	<true/>
	<key>Internal</key>
	<false/>
	<key>MediaName</key>
	<string>SD Card Reader</string>
	<key>OSInternalMedia</key>
	<false/>
	<key>RemovableMedia</key>
	<true/>
	<key>Size</key>
	<integer>63864569856</integer>
	<key>TotalSize</key>
	<integer>63864569856</integer>
	<key>VirtualOrPhysical</key>
	<string>Physical</string>
	<key>WholeDisk</key>
	<true/>
	<key>Partitions</key>
	<array><dict><key>DeviceIdentifier</key><string>disk4s1</string></dict></array>
	<key>SMARTStatus</key>
	<real>1.5</real>
</dict>
</plist>`

func mustDict(t *testing.T, s string) plistDict {
	v, err := parsePlist(strings.NewReader(s))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("not a dict: %T", v)
	}
	return plistDict(m)
}

func TestMacDiskJudge(t *testing.T) {
	d := mustDict(t, sdReaderPlist)
	if d.str("DeviceIdentifier") != "disk4" || d.num("TotalSize") != 63864569856 {
		t.Fatalf("parse: %v", d)
	}
	if b, ok := d.boolean("Ejectable"); !ok || !b {
		t.Fatal("bool")
	}
	if parts, _ := d["Partitions"].([]any); len(parts) != 1 {
		t.Fatal("array")
	}
	var info DiskInfo
	judgeMacDisk(&info, d, map[int]bool{0: true, 3: true})
	if !info.OK || info.Disk != 4 || info.Model != "SD Card Reader" || info.Bus != "USB" {
		t.Fatalf("SD card refused: %+v", info)
	}

	// the boot disk is refused
	info = DiskInfo{}
	judgeMacDisk(&info, d, map[int]bool{4: true})
	if info.OK || !strings.Contains(info.Reason, "macOS") {
		t.Fatalf("boot disk: %+v", info)
	}
	// an internal NVMe drive is refused
	internal := strings.NewReplacer("<string>USB</string>", "<string>Apple Fabric</string>",
		"<key>Internal</key>\n\t<false/>", "<key>Internal</key>\n\t<true/>",
		"<key>RemovableMedia</key>\n\t<true/>", "<key>RemovableMedia</key>\n\t<false/>",
		"<key>Ejectable</key>\n\t<true/>", "<key>Ejectable</key>\n\t<false/>").Replace(sdReaderPlist)
	info = DiskInfo{}
	judgeMacDisk(&info, mustDict(t, internal), nil)
	if info.OK || !strings.Contains(info.Reason, "internal") {
		t.Fatalf("internal: %+v", info)
	}
	// the built in SD slot of a MacBook is internal but allowed
	builtin := strings.NewReplacer("<string>USB</string>", "<string>Secure Digital</string>",
		"<key>Internal</key>\n\t<false/>", "<key>Internal</key>\n\t<true/>").Replace(sdReaderPlist)
	info = DiskInfo{}
	judgeMacDisk(&info, mustDict(t, builtin), nil)
	if !info.OK {
		t.Fatalf("built in slot: %+v", info)
	}
	// a disk image is refused
	info = DiskInfo{}
	judgeMacDisk(&info, mustDict(t, strings.Replace(sdReaderPlist, "<string>Physical</string>", "<string>Virtual</string>", 1)), nil)
	if info.OK {
		t.Fatal("disk image accepted")
	}
	// no card in the reader
	info = DiskInfo{}
	judgeMacDisk(&info, mustDict(t, strings.ReplaceAll(sdReaderPlist, "63864569856", "0")), nil)
	if info.OK {
		t.Fatal("empty reader accepted")
	}
	for in, want := range map[string]int{"disk4": 4, "/dev/disk12": 12, "disk4s1": 4, "/dev/rdisk2": 2, "sda": -1} {
		if got := macDiskNumber(in); got != want {
			t.Fatalf("macDiskNumber(%q) = %d", in, got)
		}
	}
	if macConfirmWord("gdemu") != "GDEMU" || macConfirmWord("") != "ERASE" {
		t.Fatal("confirm word")
	}
	// paths on the card
	card := &DiskInfo{Letters: []string{"/Volumes/GDEMU"}}
	if !onDisk("/Volumes/GDEMU/02/disc.gdi", card) || onDisk("/Volumes/GDEMU2/x", card) || onDisk("/Users/glen/Downloads", card) {
		t.Fatal("onDisk")
	}
}
