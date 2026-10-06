package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSmartCollections(t *testing.T) {
	root := t.TempDir()
	data := t.TempDir()
	os.MkdirAll(filepath.Join(root, editsDir), 0o755)
	c := &Card{Root: root, Games: []Game{
		{Folder: "02", Product: "RACE1", Name: "Racer", Region: "USA"},
		{Folder: "03", Product: "FIGHT1", Name: "Fighter", Region: "JPN"},
		{Folder: "04", Product: "NOINFO", Name: "Mystery", Region: "EUR"},
		{Folder: "05", Product: "RACE1", Name: "Racer disc 2", Region: "USA"},
	}}
	d := newDat(metaSize)
	d.Set("RACE1", encodeMeta(Meta{Players: 2, Genre: 1 << 1, Network: 1, Accessories: 1 << 5}))
	d.Set("FIGHT1", encodeMeta(Meta{Players: 2, Genre: 1 << 5}))
	d.Write(filepath.Join(data, "META.DAT"))
	// the owner's edit wins: Mystery becomes a 4 player fighter
	e := loadEdits(root)
	e.Games["04"] = &GameEdit{Product: "NOINFO", Meta: &Meta{Players: 4, Genre: 1 << 5}}
	e.save()
	list := []Collection{
		{Name: "Racing", Rule: &ColRule{Genres: 1 << 1}},
		{Name: "Fighters", Products: []string{"RACE1"}, Rule: &ColRule{Genres: 1 << 5}},
		{Name: "Four players", Rule: &ColRule{MinPlayers: 3}},
		{Name: "Online", Rule: &ColRule{Online: true}},
		{Name: "Wheel", Rule: &ColRule{Accessories: 1 << 5}},
		{Name: "Japan", Rule: &ColRule{Regions: []string{"jpn"}}},
		{Name: "Picked only", Products: []string{"FIGHT1"}},
		{Name: "Empty rule", Rule: &ColRule{}},
	}
	got := resolveCollections(root, c, list, filepath.Join(data, "META.DAT"))
	want := map[string]string{"Racing": "RACE1", "Fighters": "RACE1 FIGHT1 NOINFO", "Four players": "NOINFO", "Online": "RACE1", "Wheel": "RACE1", "Japan": "FIGHT1", "Picked only": "FIGHT1", "Empty rule": ""}
	for _, col := range got {
		if s := strings.Join(col.Products, " "); s != want[col.Name] {
			t.Errorf("%s: %q, want %q", col.Name, s, want[col.Name])
		}
	}
	// saving keeps the rule and drops an empty one; the text SWIRL reads lists the resolved games
	if err := SaveCollections(root, list); err != nil {
		t.Fatal(err)
	}
	saved := LoadCollections(root)
	if saved[0].Rule == nil || saved[0].Rule.Genres != 1<<1 || saved[7].Rule != nil {
		t.Fatalf("%+v", saved)
	}
	txt := collectText(resolveCollections(root, c, saved, filepath.Join(data, "META.DAT")))
	if !strings.Contains(txt, "[Racing]\r\nRACE1\r\n") || !strings.Contains(txt, "[Four players]\r\nNOINFO\r\n") {
		t.Fatal(txt)
	}
}
