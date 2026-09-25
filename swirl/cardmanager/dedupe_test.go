package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDedupe(t *testing.T) {
	root := os.Getenv("SWIRL_CARD")
	if root == "" {
		t.Skip()
	}
	logf := func(f string, a ...any) { t.Logf(f, a...) }
	// make folder 09 an exact copy of 03 and add an edit on 08 to check it follows its game
	src := filepath.Join(root, "03")
	dst := filepath.Join(root, "09")
	copyTree(src, dst)
	if os.Getenv("MIDDLE") != "" { // put the extra copy in the middle: 04 <-> 09
		os.Rename(filepath.Join(root, "04"), filepath.Join(root, "tmp"))
		os.Rename(dst, filepath.Join(root, "04"))
		os.Rename(filepath.Join(root, "tmp"), dst)
	}
	e := loadEdits(root)
	c, _ := ScanCard(root)
	for _, g := range c.Games {
		if g.Folder == "08" {
			e.Games["08"] = &GameEdit{Product: g.Product, Meta: &Meta{Description: "follow me"}}
		}
	}
	e.save()
	c, _ = ScanCard(root)
	if len(c.Dups) == 0 {
		t.Fatal("no dups found")
	}
	t.Logf("dups %+v", c.Dups)
	if err := RemoveDuplicates(root, logf); err != nil {
		t.Fatal(err)
	}
	c, _ = ScanCard(root)
	for i, g := range c.Games {
		if g.Slot != i+2 {
			t.Fatalf("gap at %s", g.Folder)
		}
	}
	if len(c.Dups) != 0 || len(c.Warnings) != 0 {
		t.Fatalf("left %+v %v", c.Dups, c.Warnings)
	}
	e = loadEdits(root)
	found := false
	for f, ge := range e.Games {
		if ge.Meta != nil && ge.Meta.Description == "follow me" {
			found = true
			t.Logf("edit followed to folder %s", f)
		}
	}
	if !found {
		t.Fatal("edit lost")
	}
	t.Logf("games now %d, backups %v", len(c.Games), c.Backups)
}
