package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestDisc is writeTestCDI for disc d of n.
func writeTestDisc(t *testing.T, path, title, serial string, d, n int) {
	writeTestCDI(t, path, title, serial)
	b, _ := os.ReadFile(path)
	b = bytes.Replace(b, []byte("CD-ROM1/1"), []byte("CD-ROM"+string(rune('0'+d))+"/"+string(rune('0'+n))), -1)
	os.WriteFile(path, b, 0o644)
}

func TestTitleLookup(t *testing.T) {
	cases := map[string]string{"MK-51064": "18 Wheeler - American Pro Trucker", "T-9708N": "4 Wheel Thunder", "MK-51035": "Crazy Taxi", "MK51059": "Shenmue", "T-17705N": "Deep Fighter"}
	for serial, want := range cases {
		if e, ok := lookupTitle(serial, 0); !ok || e.Title != want {
			t.Errorf("%s: got %+v", serial, e)
		}
	}
	if e, _ := lookupTitle("MK-51059", 3); e.Disc != 3 || e.Discs != 3 {
		t.Errorf("shenmue disc 3: %+v", e)
	}
	labels := map[string]string{
		"Crazy_Taxi_(USA)_[!]":                    "Crazy Taxi",
		"Shenmue (USA) (Disc 2)":                  "Shenmue",
		"House of the Dead 2, The (USA)":          "The House of the Dead 2",
		"SONIC ADVENTURE II":                      "Sonic Adventure II",
		"skies.of.arcadia.cd1":                    "Skies of Arcadia",
		"Marvel vs. Capcom 2 (USA) (Rev 1) - CD1": "Marvel vs. Capcom 2",
	}
	for in, want := range labels {
		if got := cleanLabel(in); got != want {
			t.Errorf("cleanLabel(%q) = %q, want %q", in, got, want)
		}
	}
	if properName(&ipInfo{Name: "18WHEELER", Product: "MK-51064", Disc: "1/1"}, "disc") != "18 Wheeler - American Pro Trucker" {
		t.Error("serial name")
	}
	if properName(&ipInfo{Name: "MY HOMEBREW", Product: "HB-0001", Disc: "1/1"}, "disc") != "My Homebrew" {
		t.Error("disc title fallback")
	}
	if properName(&ipInfo{Name: "MY HOMEBREW", Product: "HB-0001", Disc: "1/1"}, "Cool Homebrew (v1.2)") != "Cool Homebrew" {
		t.Error("label fallback")
	}
}

func TestDiscSetsAndNames(t *testing.T) {
	card := t.TempDir()
	logf := func(f string, a ...any) { t.Logf(f, a...) }
	writeTestDisc(t, filepath.Join(card, "02", "disc.cdi"), "SHENMUE", "MK-51059", 1, 3)
	writeTestCDI(t, filepath.Join(card, "03", "disc.cdi"), "CRAZYTAXI", "MK-51035")
	writeTestDisc(t, filepath.Join(card, "04", "disc.cdi"), "SHENMUE", "MK-51059", 3, 3)
	if err := installSwirl(card, "", false, logf); err != nil {
		t.Fatal(err)
	}
	c, _ := ScanCard(card)
	if len(c.Sets) != 1 || !c.Sets[0].Apart || strings.Join(c.Sets[0].Folders, ",") != "02,04" || len(c.Sets[0].Missing) != 1 || c.Sets[0].Missing[0] != 2 {
		t.Fatalf("sets %+v", c.Sets)
	}
	if c.Games[0].Suggested != "Shenmue" || c.Games[1].Suggested != "Crazy Taxi" {
		t.Fatalf("suggestions %+v", c.Games)
	}

	// adding disc 2 from a download: named properly and put between discs 1 and 3
	src := t.TempDir()
	writeTestDisc(t, filepath.Join(src, "shenmue_us_disc2", "sm2.cdi"), "SHENMUE", "MK-51059", 2, 3)
	if err := StartAddGames(card, []string{filepath.Join(src, "shenmue_us_disc2")}, ""); err != nil {
		t.Fatal(err)
	}
	if j := waitJob(t); j.Error != "" {
		t.Fatal(j.Error)
	}
	c, _ = ScanCard(card)
	var got []string
	for _, g := range c.Games {
		got = append(got, g.Folder+":"+g.Name+":"+g.Disc)
	}
	if strings.Join(got, ",") != "02:SHENMUE:1/3,03:Shenmue:2/3,04:SHENMUE:3/3,05:CRAZYTAXI:1/1" {
		t.Fatalf("after add %v", got)
	}
	if len(c.Sets) != 1 || c.Sets[0].Apart || len(c.Sets[0].Missing) != 0 {
		t.Fatalf("sets after add %+v", c.Sets)
	}

	// a name typed in the Edit window is never suggested away
	if err := SaveGame(SaveGameRequest{Root: card, Folder: "05", Name: "Taxi!", Region: "JUE", VGA: true}); err != nil {
		t.Fatal(err)
	}
	c, _ = ScanCard(card)
	if c.Games[3].Name != "Taxi!" || c.Games[3].Suggested != "" || !c.Games[3].UserName {
		t.Fatalf("user name %+v", c.Games[3])
	}
}
