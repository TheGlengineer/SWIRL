package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// UP-3: the Bundle's virtual folders become collections on the first install, and its type.txt and
// disc.txt are carried into the menu's list. A card in database mode (no sidecar files) is read from
// DISCDB.JSON instead.
func TestVFBFoldersAndTypes(t *testing.T) {
	root := txnCard(t, "openMenu", 5)
	w := func(folder, name, text string) {
		os.WriteFile(filepath.Join(root, folder, name), []byte(text+"\r\n"), 0o644)
	}
	w("02", "folder.txt", "Racing")
	w("02", "folder_alt1.txt", "Arcade")
	w("03", "folder.txt", "racing") // same folder, different case
	w("04", "type.txt", "other")
	w("04", "disc.txt", "2/3")
	w("05", "type.txt", "game")
	w("05", "disc.txt", "not a disc number")
	vfbDB(t, root, map[string][2]string{"06": {"T00006N", "GAME E"}})
	// 06 has no sidecars; its database entry says psx, folder RPG and the alt folder Arcade
	b, _ := os.ReadFile(filepath.Join(root, discDBName))
	s := strings.Replace(string(b), `"type": "game"`, `"type": "psx"`, 1)
	s = strings.Replace(s, `"folder": "Racing"`, `"folder": "RPG"`, 1)
	os.WriteFile(filepath.Join(root, discDBName), []byte(s), 0o644)

	c, err := ScanCard(root)
	if err != nil {
		t.Fatal(err)
	}
	if g := findGame(c, "04"); g.Type != "other" || g.Disc != "2/3" || g.DiscNo != 2 || g.DiscOf != 3 {
		t.Fatalf("04: type %q disc %q", g.Type, g.Disc)
	}
	if g := findGame(c, "05"); g.Type != "" || g.Disc != "1/1" {
		t.Fatalf("05: type %q disc %q", g.Type, g.Disc)
	}
	if g := findGame(c, "06"); g.Type != "psx" {
		t.Fatalf("06: type %q", g.Type)
	}
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	cols := LoadCollections(root)
	got := map[string]string{}
	for _, col := range cols {
		got[col.Name] = strings.Join(col.Products, ",")
	}
	if len(cols) != 3 || got["Racing"] != "T00002N,T00003N" || got["Arcade"] != "T00002N,T00006N" || got["RPG"] != "T00006N" {
		t.Fatalf("collections %v", got)
	}
	ini, err := menuINI(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"04.type=other", "04.disc=2/3", "06.type=psx"} {
		if !strings.Contains(ini, want) {
			t.Fatalf("INI lacks %s:\n%s", want, ini)
		}
	}
	m, _ := openMenuDisc(root)
	_, collect := m.files["COLLECT.TXT"]
	m.Close()
	if !collect {
		t.Fatal("COLLECT.TXT is not on the menu disc")
	}
	// the owner removes every collection; the next install does not bring the folders back
	if err := SaveCollections(root, nil); err != nil {
		t.Fatal(err)
	}
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	if n := len(LoadCollections(root)); n != 0 {
		t.Fatalf("%d collections came back", n)
	}
	// a card with no virtual folders gets no collections and no marker
	root2 := txnCard(t, "openMenu", 2)
	if err := InstallSwirl(root2, "", quiet); err != nil {
		t.Fatal(err)
	}
	if len(LoadCollections(root2)) != 0 || fileExists(vfbFoldersMarker(root2)) {
		t.Fatal("collections made from nothing")
	}
}

func TestVFBTooManyFolders(t *testing.T) {
	root := txnCard(t, "openMenu", 3)
	for i, f := range []string{"02", "03", "04"} {
		for j := 0; j < 5; j++ {
			name := "folder_alt" + string(rune('1'+j)) + ".txt"
			if j == 0 {
				name = "folder.txt"
			}
			os.WriteFile(filepath.Join(root, f, name), []byte("Folder "+string(rune('A'+i*5+j))+" and more words"), 0o644)
		}
	}
	// 15 folders with one game each, plus 12 more on 02 through the alt files is not possible, so 15 it is
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	if n := len(LoadCollections(root)); n != 15 {
		t.Fatalf("%d collections", n)
	}
}
