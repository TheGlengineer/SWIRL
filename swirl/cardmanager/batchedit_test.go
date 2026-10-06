package main

import (
	"testing"
)

func TestBatchEdit(t *testing.T) {
	root := txnCard(t, "", 3)
	e := loadEdits(root)
	e.Games["02"].Meta = &Meta{Players: 1, Genre: 1 << 2, Description: "Keep me"}
	e.Games["02"].Desc = map[string]string{"de": "Behalte mich"}
	e.save()
	vga := false
	players := 4
	n, err := ApplyBatchEdit(BatchEdit{Root: root, Folders: []string{"02", "03", "99"}, Region: "eur", VGA: &vga, Players: &players, Genres: 1 << 1})
	if err != nil || n != 2 {
		t.Fatal(n, err)
	}
	e = loadEdits(root)
	a, b := e.Games["02"], e.Games["03"]
	if a.Region != "EUR" || *a.VGA || a.Meta.Players != 4 || a.Meta.Genre != 1<<2|1<<1 || a.Meta.Description != "Keep me" || a.Desc["de"] != "Behalte mich" {
		t.Fatalf("%+v %+v", a, a.Meta)
	}
	if b.Region != "EUR" || b.Meta == nil || b.Meta.Players != 4 || b.Meta.Genre != 1<<1 {
		t.Fatalf("%+v %+v", b, b.Meta)
	}
	c, _ := ScanCard(root)
	if g := findGame(c, "02"); g.Region != "EUR" || g.VGA || !g.Edited {
		t.Fatalf("%+v", g)
	}
	if _, err := ApplyBatchEdit(BatchEdit{Root: root, Folders: []string{"02"}}); err == nil {
		t.Fatal("empty change accepted")
	}
	if _, err := ApplyBatchEdit(BatchEdit{Root: root, Folders: []string{"02"}, ClearGenres: true}); err != nil {
		t.Fatal(err)
	}
	if loadEdits(root).Games["02"].Meta.Genre != 0 {
		t.Fatal("genres not cleared")
	}
}
