package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPendingChanges(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, editsDir, "art"), 0o755)
	c := &Card{Root: root, Games: []Game{{Folder: "02", Name: "A", Product: "T1"}}}
	if p := PendingChanges(root, c); p.Known || len(p.Sections) != 0 {
		t.Fatalf("no stamp: %+v", p)
	}
	st := buildStamp{Version: version, Sections: menuInputs(root, c)}
	writeStamp := func() {
		st.Sections = menuInputs(root, c)
		bb, _ := json.Marshal(st)
		os.WriteFile(menuStampPath(root), bb, 0o644)
	}
	writeStamp()
	if p := PendingChanges(root, c); !p.Known || len(p.Sections) != 0 {
		t.Fatalf("fresh stamp: %+v", p)
	}
	os.WriteFile(filepath.Join(root, editsDir, "games.json"), []byte(`{"games":{}}`), 0o644)
	c.Games[0].Name = "B"
	os.WriteFile(filepath.Join(root, editsDir, "art", "02_box.png"), []byte("png"), 0o644)
	p := PendingChanges(root, c)
	if !p.Known || len(p.Sections) != 3 || p.Sections[0] != "games" || p.Sections[1] != "edits" || p.Sections[2] != "art" {
		t.Fatalf("%+v", p)
	}
	writeStamp()
	if p := PendingChanges(root, c); len(p.Sections) != 0 {
		t.Fatalf("after restamp: %+v", p)
	}
	os.WriteFile(filepath.Join(root, editsDir, "collections.json"), []byte(`[]`), 0o644)
	if p := PendingChanges(root, c); len(p.Sections) != 1 || p.Sections[0] != "collections" {
		t.Fatalf("%+v", p)
	}
}
