package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCardWithPickedGames(t *testing.T) {
	src := t.TempDir()
	writeTestGDI(t, filepath.Join(src, "w", "Zip Racer (USA)"), "ZIP RACER", "T-00010N")
	zipDir(t, filepath.Join(src, "w", "Zip Racer (USA)"), filepath.Join(src, "Zip Racer (USA).zip"))
	writeTestCDI(t, filepath.Join(src, "crazy_taxi_usa.cdi"), "CRAZYTAXI", "MK-51035")
	writeTestCDI(t, filepath.Join(src, "Other Folder", "game", "x.cdi"), "OTHER", "T-00020N")
	writeTestCDI(t, filepath.Join(src, "not picked.cdi"), "NOPE", "T-00099N")

	card := t.TempDir()
	formatCardHook = func(root string, disk int, report func(float64, string)) (string, error) { return card, nil }
	defer func() { formatCardHook = formatCard }()
	req := NewCardRequest{Picks: []string{
		filepath.Join(src, "Zip Racer (USA).zip"),
		filepath.Join(src, "crazy_taxi_usa.cdi"),
		filepath.Join(src, "Other Folder", "game"),
		filepath.Join(src, "crazy_taxi_usa.cdi"), // picked twice
	}}
	plan, err := planCopy("")
	if err != nil {
		t.Fatal(err)
	}
	job = jobState{Running: true}
	runNewCard(req, &DiskInfo{Root: card, Model: "test card", SizeBytes: 4 << 30}, plan)
	if job.Error != "" {
		t.Fatal(job.Error, job.Log)
	}
	t.Log(strings.Join(job.Log, "\n"))
	c, err := ScanCard(card)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, g := range c.Games {
		got = append(got, g.Folder+":"+g.Name)
	}
	if c.MenuType != "SWIRL" || strings.Join(got, ",") != "02:Zip Racer,03:Crazy Taxi,04:Other" {
		t.Fatalf("card %s %v", c.MenuType, got)
	}
	if fileExists(filepath.Join(card, "05")) {
		t.Fatal("the game picked twice was copied twice")
	}
}

func TestEmptyCardScansAsLists(t *testing.T) {
	card := t.TempDir()
	if err := installSwirl(card, "", true, t.Logf); err != nil {
		t.Fatal(err)
	}
	c, err := ScanCard(card)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(c)
	for _, k := range []string{`"games":[]`, `"warnings":[]`, `"backups":[]`, `"duplicates":[]`, `"sets":[]`} {
		if !strings.Contains(string(b), k) {
			t.Fatalf("%s missing in %s", k, b)
		}
	}
	// Update SWIRL on it works without games
	if err := InstallSwirl(card, "", t.Logf); err != nil {
		t.Fatal("update on an empty card:", err)
	}
	// but not on a drive that has no menu and no games
	if err := InstallSwirl(t.TempDir(), "", t.Logf); err == nil {
		t.Fatal("installed on a blank drive")
	}
	_ = os.Remove
}
