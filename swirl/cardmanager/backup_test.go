package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitJobAny(t *testing.T) jobState {
	for i := 0; i < 3000; i++ {
		if j := jobSnapshot(); j.Done {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job timeout")
	return jobState{}
}

func TestBackupToPC(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "FIRST GAME", "T-00001N")
	writeTestCDI(t, filepath.Join(card, "03", "disc.cdi"), "SECOND GAME", "T-00002N")
	logf := func(f string, a ...any) {}
	if err := installSwirl(card, "", false, logf); err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(card, "SWIRL_BACKUP", "old"), 0o755)
	os.WriteFile(filepath.Join(card, "SWIRL_BACKUP", "old", "x.bin"), []byte("old"), 0o644)
	os.MkdirAll(filepath.Join(card, "System Volume Information"), 0o755)
	os.WriteFile(filepath.Join(card, "System Volume Information", "IndexerVolumeGuid"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(card, "03", "._disc.cdi"), []byte("mac junk"), 0o644)

	// a folder on the card is refused
	if err := StartBackup(BackupRequest{Root: card, Dest: filepath.Join(card, "bk")}); err == nil {
		t.Fatal("a destination on the card was accepted")
	}

	dest := t.TempDir()
	if err := StartBackup(BackupRequest{Root: card, Dest: dest}); err != nil {
		t.Fatal(err)
	}
	j := waitJobAny(t)
	if j.Error != "" {
		t.Fatal(j.Error, j.Log)
	}
	list := ListBackups(dest)
	if len(list) != 1 || !list[0].Info.Complete || list[0].Info.Games != 2 || list[0].Info.CardID == "" {
		t.Fatalf("backups %+v", list)
	}
	bk := list[0].Path
	if !strings.HasPrefix(filepath.Base(bk), "SWIRL card backup ") {
		t.Fatal("folder name", bk)
	}
	for _, f := range []string{"02/disc.cdi", "03/disc.cdi", "SWIRL/card-id.txt"} {
		if !fileExists(filepath.Join(bk, f)) {
			t.Fatal("missing", f)
		}
	}
	if !isDir(filepath.Join(bk, "01")) {
		t.Fatal("menu folder not backed up")
	}
	for _, f := range []string{"SWIRL_BACKUP", "System Volume Information", "03/._disc.cdi"} {
		if fileExists(filepath.Join(bk, f)) || isDir(filepath.Join(bk, f)) {
			t.Fatal("should have been skipped:", f)
		}
	}
	a, _ := os.ReadFile(filepath.Join(card, "02", "disc.cdi"))
	b, _ := os.ReadFile(filepath.Join(bk, "02", "disc.cdi"))
	if string(a) != string(b) {
		t.Fatal("copy differs")
	}
	st1, _ := os.Stat(filepath.Join(card, "02", "disc.cdi"))
	st2, _ := os.Stat(filepath.Join(bk, "02", "disc.cdi"))
	if st1.ModTime().Sub(st2.ModTime()).Abs() > 2*time.Second {
		t.Fatal("modified time not kept")
	}

	// update: one changed file, one removed game, one new file; SWIRL_BACKUP now included
	os.WriteFile(filepath.Join(card, "03", "extra.txt"), []byte("new"), 0o644)
	os.RemoveAll(filepath.Join(card, "02"))
	if err := StartBackup(BackupRequest{Root: card, Dest: dest, Update: true, IncludeOld: true}); err != nil {
		t.Fatal(err)
	}
	j = waitJobAny(t)
	if j.Error != "" {
		t.Fatal(j.Error, j.Log)
	}
	t.Log(strings.Join(j.Log, "\n"))
	if l := ListBackups(dest); len(l) != 1 || l[0].Path != bk {
		t.Fatalf("update made a new folder: %+v", l)
	}
	if isDir(filepath.Join(bk, "02")) || !fileExists(filepath.Join(bk, "03", "extra.txt")) || !fileExists(filepath.Join(bk, "SWIRL_BACKUP", "old", "x.bin")) {
		t.Fatal("update did not mirror the card")
	}
	if !strings.Contains(strings.Join(j.Log, "\n"), "copying 2 (") {
		t.Fatal("update copied everything again")
	}

	// a different card never updates this backup
	other := t.TempDir()
	writeTestCDI(t, filepath.Join(other, "02", "disc.cdi"), "OTHER", "T-00009N")
	os.MkdirAll(filepath.Join(other, "01"), 0o755)
	if err := StartBackup(BackupRequest{Root: other, Dest: dest, Update: true}); err != nil {
		t.Fatal(err)
	}
	if j = waitJobAny(t); j.Error != "" {
		t.Fatal(j.Error)
	}
	if l := ListBackups(dest); len(l) != 2 {
		t.Fatalf("other card: %d backups", len(l))
	}

	// cancel: stops, and the marker says incomplete
	big := t.TempDir()
	os.MkdirAll(filepath.Join(big, "01"), 0o755)
	for i := 0; i < 40; i++ {
		os.WriteFile(filepath.Join(big, "01", "f"+string(rune('a'+i%26))+string(rune('a'+i/26))+".bin"), make([]byte, 8<<20), 0o644)
	}
	dest2 := t.TempDir()
	if err := StartBackup(BackupRequest{Root: big, Dest: dest2}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200 && !jobSnapshot().Cancellable; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	jobCancel.Store(true)
	j = waitJobAny(t)
	if !strings.Contains(j.Error, "incomplete") {
		t.Fatalf("cancel: %q", j.Error)
	}
	if l := ListBackups(dest2); len(l) != 1 || l[0].Info.Complete {
		t.Fatalf("cancelled backup %+v", l)
	}
	jobCancel.Store(false)
}
