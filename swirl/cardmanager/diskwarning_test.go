package main

import "testing"

func TestDiskWarning(t *testing.T) {
	cases := []struct {
		model string
		size  uint64
		warn  bool
	}{
		{"Generic STORAGE DEVICE", 128 << 30, false},
		{"SanDisk Ultra", 256 << 30, true}, // the brand makes SSDs too, so a bare brand name is checked by the person
		{"Transcend SD Card Reader", 256 << 30, false},
		{"Realtek USB3.0 Card Reader", 64 << 30, false},
		{"Unknown card", 32 << 30, false},
		{"Samsung SSD 870 EVO", 1 << 40, true},    // too big for a card
		{"WDC WD10EZEX-00BN5A0", 200 << 30, true}, // a hard disk's name
		{"Seagate Expansion", 500 << 30, true},
	}
	for _, c := range cases {
		info := &DiskInfo{Model: c.model, SizeBytes: c.size, OK: true}
		diskWarning(info)
		if (info.Warning != "") != c.warn {
			t.Errorf("%s %d GB: warning %q", c.model, c.size>>30, info.Warning)
		}
	}
	// a disk that failed the checks gets no warning on top
	info := &DiskInfo{Model: "WDC", SizeBytes: 1 << 40, Reason: "no"}
	diskWarning(info)
	if info.Warning != "" {
		t.Error("warning on a refused disk")
	}
}
