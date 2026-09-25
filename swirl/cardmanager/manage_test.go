package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitJob(t *testing.T) jobState {
	for i := 0; i < 600; i++ {
		j := jobSnapshot()
		if j.Done {
			if j.Error != "" {
				t.Fatalf("job failed: %s\n%v", j.Error, j.Log)
			}
			return j
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("job timeout")
	return jobState{}
}

func waitJobLong(t *testing.T) jobState {
	for i := 0; i < 3000; i++ {
		j := jobSnapshot()
		if j.Done {
			if j.Error != "" {
				t.Fatalf("job failed: %s\n%v", j.Error, j.Log)
			}
			return j
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("job timeout")
	return jobState{}
}

func TestManageGames(t *testing.T) {
	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "GAME A", "T-00001N")
	writeTestCDI(t, filepath.Join(card, "03", "disc.cdi"), "GAME B", "T-00002N")
	writeTestCDI(t, filepath.Join(card, "04", "disc.cdi"), "GAME C", "T-00003N")
	logf := func(f string, a ...any) { t.Logf(f, a...) }
	// edits for A and B so we can see they follow a swap
	s := loadEdits(card)
	s.Games["02"] = &GameEdit{Product: "T00001N", Date: "20050505"}
	s.Games["03"] = &GameEdit{Product: "T00002N", Date: "20020202"}
	s.save()
	if err := installSwirl(card, "", false, logf); err != nil {
		t.Fatal(err)
	}

	// reorder: C, A, B
	if err := StartReorder(card, []string{"04", "02", "03"}); err != nil {
		t.Fatal(err)
	}
	waitJob(t)
	c, _ := ScanCard(card)
	got := []string{}
	for _, g := range c.Games {
		got = append(got, g.Name+"@"+g.Date)
	}
	if strings.Join(got, ",") != "GAME C@20010101,GAME A@20050505,GAME B@20020202" {
		t.Fatalf("after reorder %v", got)
	}

	// add: a named folder, a loose cdi, and a gdi file with its tracks
	src := t.TempDir()
	writeTestCDI(t, filepath.Join(src, "Named Game", "x.cdi"), "GAME D", "T-00004N")
	writeTestCDI(t, filepath.Join(src, "Loose.cdi"), "GAME E", "T-00005N")
	gd := filepath.Join(src, "gdigame")
	os.MkdirAll(gd, 0o755)
	os.WriteFile(filepath.Join(gd, "disc.gdi"), []byte("3\n1 0 4 2352 track01.bin 0\n2 600 0 2352 \"track 02.raw\" 0\n3 45000 4 2352 track03.bin 0\n"), 0o644)
	for _, n := range []string{"track01.bin", "track 02.raw", "track03.bin"} {
		os.WriteFile(filepath.Join(gd, n), []byte("x"), 0o644)
	}
	if _, err := planAdd(card, []string{filepath.Join(src, "nope.cdi")}); err == nil {
		t.Fatal("missing file accepted")
	}
	if err := StartAddGames(card, []string{filepath.Join(src, "Named Game"), filepath.Join(src, "Loose.cdi"), filepath.Join(gd, "disc.gdi")}, ""); err != nil {
		t.Fatal(err)
	}
	waitJob(t)
	for _, f := range []string{"05/x.cdi", "05/name.txt", "06/Loose.cdi", "07/disc.gdi", "07/track 02.raw", "07/track03.bin"} {
		if !fileExists(filepath.Join(card, f)) {
			t.Fatal("missing after add:", f)
		}
	}
	c, _ = ScanCard(card)
	if len(c.Games) != 6 || c.Games[3].Name != "Named Game" || c.Games[4].Name != "Loose" {
		t.Fatalf("after add %+v", c.Games)
	}

	// remove A (now folder 03) and the loose one (06)
	if err := StartRemoveGames(card, []string{"03", "06"}); err != nil {
		t.Fatal(err)
	}
	waitJob(t)
	c, _ = ScanCard(card)
	got = got[:0]
	for _, g := range c.Games {
		got = append(got, g.Folder+":"+g.Name+"@"+g.Date)
	}
	if strings.Join(got, ",") != "02:GAME C@20010101,03:GAME B@20020202,04:Named Game@20010101,05:Gdigame@20000101" {
		t.Fatalf("after remove %v", got)
	}
	t.Logf("after remove %v", got)
	if c.MenuType != "SWIRL" {
		t.Fatal("menu", c.MenuType)
	}
}

func TestGDEMUIni(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "gdemu.ini"), []byte("open_time = 150\r\n; my note\r\nfoo=bar\r\nreset_goto=0\r\n"), 0o644)
	s, err := ReadGDEMU(root)
	if err != nil || !s.Exists || s.OpenTime != "150" || s.ResetGoto != "0" || s.Other != "; my note\nfoo=bar" {
		t.Fatalf("%+v %v", s, err)
	}
	s.ResetGoto = "1"
	s.ReadLimit = "-1"
	if err := SaveGDEMU(root, *s); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(root, "gdemu.ini"))
	if string(b) != "open_time = 150\r\nreset_goto = 1\r\nread_limit = -1\r\n; my note\r\nfoo=bar\r\n" {
		t.Fatalf("%q", b)
	}
	s.OpenTime = "20"
	if SaveGDEMU(root, *s) == nil {
		t.Fatal("bad value accepted")
	}
}

func TestOnlineDBMerge(t *testing.T) {
	// SWIRL_DB_DIR=folder holding BOX.DAT, ICON.DAT and META.DAT from the openMenu databases
	db := os.Getenv("SWIRL_DB_DIR")
	if db == "" || !fileExists(filepath.Join(db, "BOX.DAT")) {
		t.Skip("set SWIRL_DB_DIR to a downloaded database")
	}
	dbDirOverride = db
	defer func() { dbDirOverride = "" }()
	b, _ := json.Marshal(dbInfo{Downloaded: time.Now()})
	os.WriteFile(filepath.Join(db, "info.json"), b, 0o644)
	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "SOME GAME", "T-8108N")
	writeTestCDI(t, filepath.Join(card, "03", "disc.cdi"), "NOT IN DB", "ZZ-99999")
	st := GetDBStatus(card)
	if !st.Present || st.CanFillArt != 1 {
		t.Fatalf("status %+v", st)
	}
	if err := installSwirl(card, "", false, func(f string, a ...any) { t.Logf(f, a...) }); err != nil {
		t.Fatal(err)
	}
	c, _ := ScanCard(card)
	if !c.Games[0].HasArt || c.Games[1].HasArt {
		t.Fatalf("art %+v", c.Games)
	}
	m, _ := openMenuDisc(card)
	defer m.Close()
	bb, err := m.datChunk("BOX.DAT", "T8108N")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePVR(bb); err != nil {
		t.Fatal("pvr", err)
	}
	if mb, err := m.datChunk("META.DAT", "T8108N"); err == nil {
		t.Logf("meta: %s", decodeMeta(mb).Description)
	}
}
