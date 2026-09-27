package main

import (
	"strings"
	"testing"
)

func TestParseSysfsBlockLink(t *testing.T) {
	cases := map[string][2]string{
		"../../devices/pci0000:00/0000:00:1d.0/usb2/2-1/2-1:1.0/host6/target6:0:0/6:0:0:0/block/sdb/sdb1": {"sdb", "sdb1"},
		"../../devices/pci0000:00/0000:00:14.0/usb1/1-1/1-1:1.0/host7/target7:0:0/7:0:0:0/block/sdb":      {"sdb", ""},
		"../../devices/pci0000:00/0000:00:06.0/nvme/nvme0/nvme0n1/nvme0n1p1":                              {"nvme0n1", "nvme0n1p1"},
		"../../devices/platform/soc/fa100000.mmc/mmc_host/mmc0/mmc0:59ba/block/mmcblk0/mmcblk0p1":         {"mmcblk0", "mmcblk0p1"},
	}
	for link, want := range cases {
		whole, part := parseSysfsBlockLink(link)
		if whole != want[0] || part != want[1] {
			t.Errorf("parseSysfsBlockLink(%q) = (%q, %q), want (%q, %q)", link, whole, part, want[0], want[1])
		}
	}
}

func TestPartDev(t *testing.T) {
	cases := map[string]string{"sdb": "sdb1", "mmcblk0": "mmcblk0p1", "nvme0n1": "nvme0n1p1"}
	for in, want := range cases {
		if got := partDev(in); got != want {
			t.Errorf("partDev(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLinuxConfirmWord(t *testing.T) {
	if linuxConfirmWord("sdb") != "CONFIRM" || linuxConfirmWord("") != "CONFIRM" {
		t.Fatal("confirm word should always be CONFIRM on Linux")
	}
}

func TestJudgeLinuxDisk(t *testing.T) {
	usb := linuxBlockDev{Name: "sdb", Model: "SD Card Reader", USB: true, SizeBytes: 63864569856, SecSize: 512}
	var info DiskInfo
	judgeLinuxDisk(&info, usb, "sda")
	if !info.OK || info.Disk == 0 || info.Model != "SD Card Reader" || info.Bus != "USB" {
		t.Fatalf("SD card refused: %+v", info)
	}
	if !info.Removable {
		t.Fatalf("USB device not marked removable: %+v", info)
	}

	// the disk holding / is refused
	info = DiskInfo{}
	judgeLinuxDisk(&info, usb, "sdb")
	if info.OK || !strings.Contains(info.Reason, "Linux") {
		t.Fatalf("root disk: %+v", info)
	}

	// a built-in SD slot (mmc, not USB, marked removable) is allowed
	builtin := linuxBlockDev{Name: "mmcblk0", Model: "SD", Removable: true, SizeBytes: 63864569856, SecSize: 512}
	info = DiskInfo{}
	judgeLinuxDisk(&info, builtin, "sda")
	if !info.OK {
		t.Fatalf("built in slot refused: %+v", info)
	}

	// an internal SATA disk is refused
	internal := linuxBlockDev{Name: "sda", Model: "Samsung SSD 850", USB: false, Removable: false, SizeBytes: 500 << 30, SecSize: 512}
	info = DiskInfo{}
	judgeLinuxDisk(&info, internal, "nvme0n1")
	if info.OK || !strings.Contains(info.Reason, "internal") {
		t.Fatalf("internal: %+v", info)
	}

	// wrong sector size
	odd := linuxBlockDev{Name: "sdb", Model: "SD", USB: true, SizeBytes: 63864569856, SecSize: 4096}
	info = DiskInfo{}
	judgeLinuxDisk(&info, odd, "sda")
	if info.OK || !strings.Contains(info.Reason, "sectors") {
		t.Fatalf("4k sectors: %+v", info)
	}

	// no card in the reader
	empty := linuxBlockDev{Name: "sdb", Model: "SD Card Reader", USB: true, SizeBytes: 0, SecSize: 512}
	info = DiskInfo{}
	judgeLinuxDisk(&info, empty, "sda")
	if info.OK || !strings.Contains(info.Reason, "size") {
		t.Fatalf("empty reader: %+v", info)
	}

	// a huge disk is refused
	huge := linuxBlockDev{Name: "sdb", Model: "Disk", USB: true, SizeBytes: 3 << 40, SecSize: 512}
	info = DiskInfo{}
	judgeLinuxDisk(&info, huge, "sda")
	if info.OK || !strings.Contains(info.Reason, "2 TB") {
		t.Fatalf("huge disk: %+v", info)
	}

	// a tiny disk is refused
	tiny := linuxBlockDev{Name: "sdb", Model: "Disk", USB: true, SizeBytes: 64 << 20, SecSize: 512}
	info = DiskInfo{}
	judgeLinuxDisk(&info, tiny, "sda")
	if info.OK || !strings.Contains(info.Reason, "small") {
		t.Fatalf("tiny disk: %+v", info)
	}

	// paths on the card
	card := &DiskInfo{Letters: []string{"/media/james/SWIRL"}}
	if !onDisk("/media/james/SWIRL/02/disc.gdi", card) || onDisk("/media/james/SWIRL2/x", card) || onDisk("/home/james/Downloads", card) {
		t.Fatal("onDisk")
	}
}
