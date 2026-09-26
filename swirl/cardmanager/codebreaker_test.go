package main

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCodeBreaker(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", os.Getenv("LOCALAPPDATA"))
	t.Setenv("HOME", os.Getenv("LOCALAPPDATA"))
	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, f) }

	// the owner's CodeBreaker files: PELICAN.BIN with a CHEATS folder next to it
	src := t.TempDir()
	pelican := make([]byte, 200<<10)
	rand.New(rand.NewSource(1)).Read(pelican)
	os.WriteFile(filepath.Join(src, "PELICAN.BIN"), pelican, 0o644)
	os.MkdirAll(filepath.Join(src, "cheats"), 0o755)
	os.WriteFile(filepath.Join(src, "cheats", "fcdcheats.bin"), []byte("all cheats"), 0o644)
	os.WriteFile(filepath.Join(src, "cheats", "T-8101N.bin"), []byte("one game"), 0o644)
	os.WriteFile(filepath.Join(src, "cheats", "readme.txt"), []byte("not a cheat"), 0o644)

	// files that are not CodeBreaker are refused
	for name, body := range map[string][]byte{
		"tiny.bin":  []byte("x"),
		"swirl.bin": append(bytes.Repeat([]byte{1}, 20<<10), []byte("SWIRL: %d games")...),
		"image.bin": append([]byte("SEGA SEGAKATANA "), bytes.Repeat([]byte{2}, 40<<10)...),
		"zero.bin":  make([]byte, 40<<10),
	} {
		p := filepath.Join(t.TempDir(), name)
		os.WriteFile(p, body, 0o644)
		if err := SetCodeBreaker(t.TempDir(), p, logf); err == nil {
			t.Fatalf("%s was accepted as CodeBreaker", name)
		}
	}

	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "A GAME", "T-8101N")
	if GetCodeBreakerInfo(card).Present {
		t.Fatal("present before adding")
	}
	if err := SetCodeBreaker(card, filepath.Join(src, "PELICAN.BIN"), logf); err != nil {
		t.Fatal(err)
	}
	if ci := GetCodeBreakerInfo(card); !ci.Present || ci.Cheats != 2 || ci.Bytes != int64(len(pelican)) {
		t.Fatalf("info %+v", ci)
	}
	if err := installSwirl(card, "", false, logf); err != nil {
		t.Fatal(err)
	}
	b, err := readDiscFile(filepath.Join(card, "01"), "PELICAN.BIN", 32<<20)
	if err != nil || !bytes.Equal(b, pelican) {
		t.Fatal("PELICAN.BIN not on the menu disc", err)
	}
	d, err := openGDI(findGDI(filepath.Join(card, "01")))
	if err != nil {
		t.Fatal(err)
	}
	files, _ := listISO(d)
	found := map[string]string{}
	for _, f := range files {
		if strings.HasPrefix(strings.ToUpper(f.Path), "CHEATS/") && !f.Dir {
			b, _ := d.readSectors(f.LBA, (f.Size+sectorSize-1)/sectorSize)
			found[strings.ToUpper(f.Path)] = string(b[:f.Size])
		}
	}
	d.Close()
	// the disc writer stores "-" as "_"; SWIRL looks for both spellings when it starts CodeBreaker
	if found["CHEATS/T_8101N.BIN"] != "one game" || found["CHEATS/FCDCHEATS.BIN"] != "all cheats" || len(found) != 2 {
		t.Fatalf("cheat files on the menu disc: %v", found)
	}
	if !strings.Contains(strings.Join(logs, "\n"), "Added CodeBreaker") {
		t.Fatal("not logged")
	}

	// removing it takes it off the next menu disc
	if err := RemoveCodeBreaker(card); err != nil || GetCodeBreakerInfo(card).Present {
		t.Fatal("not removed", err)
	}
	// (the old disc's copy is kept by design when the owner has none set; remove it from the old disc to check)
}
