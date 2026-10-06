package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDropAdd(t *testing.T) {
	dbDirOverride = filepath.Join(t.TempDir(), "db")
	defer func() { dbDirOverride = "" }()
	root := txnCard(t, "swirl", 2)
	src := t.TempDir()
	writeTestCDI(t, filepath.Join(src, "disc.cdi"), "DROPPED GAME", "T-77777N")
	cdi, _ := os.ReadFile(filepath.Join(src, "disc.cdi"))
	id, err := newDrop()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dropPath(id, "../escape.cdi"); err == nil {
		t.Fatal("path escape accepted")
	}
	if _, err := dropPath(id, "C:/x.cdi"); err == nil {
		t.Fatal("drive letter accepted")
	}
	n, err := receiveDrop(id, "Dropped Game/disc.cdi", bytes.NewReader(cdi))
	if err != nil || int(n) != len(cdi) {
		t.Fatal(n, err)
	}
	if err := StartAddDropped(root, id, ""); err != nil {
		t.Fatal(err)
	}
	j := waitJob(t)
	if j.Error != "" {
		t.Fatal(j.Error)
	}
	c, _ := ScanCard(root)
	if len(c.Games) != 3 || c.Games[2].Name != "Dropped Game" {
		t.Fatalf("%+v", c.Games)
	}
	if _, err := os.Stat(filepath.Join(stagingDir(), id)); !os.IsNotExist(err) {
		t.Fatal("staging folder kept")
	}
	// an empty drop is refused and cleaned up
	id2, _ := newDrop()
	if err := StartAddDropped(root, id2, ""); err == nil {
		t.Fatal("empty drop accepted")
	}
	if _, err := os.Stat(filepath.Join(stagingDir(), id2)); !os.IsNotExist(err) {
		t.Fatal("empty staging folder kept")
	}
}
