package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestCDI(t *testing.T, path, title, serial string) {
	dir := t.TempDir()
	data := filepath.Join(dir, "d")
	os.MkdirAll(data, 0o755)
	os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), make([]byte, 5000), 0o644)
	iso := filepath.Join(dir, "s.iso")
	buildISO(data, iso, 11702, "T", ipSector(title, serial))
	raw, _ := os.ReadFile(iso)
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, _ := os.Create(path)
	f.Write(make([]byte, 2352*300))
	for i := 0; i < len(raw)/2048; i++ {
		f.Write(make([]byte, 8))
		f.Write(raw[i*2048 : (i+1)*2048])
		f.Write(make([]byte, 280))
	}
	f.Close()
}

func TestNewCardFlow(t *testing.T) {
	src := t.TempDir()
	writeTestCDI(t, filepath.Join(src, "02", "disc.cdi"), "GAME A", "T-00001N")
	writeTestCDI(t, filepath.Join(src, "05", "disc.cdi"), "GAME B", "T-00002N")
	writeTestCDI(t, filepath.Join(src, "Zeta Game", "zeta.cdi"), "GAME Z", "T-00003N")
	writeTestCDI(t, filepath.Join(src, "Loose One.cdi"), "GAME L", "T-00004N")
	os.MkdirAll(filepath.Join(src, "SWIRL_BACKUP", "junk"), 0o755)
	os.MkdirAll(filepath.Join(src, "empty folder"), 0o755)
	p, err := planCopy(src)
	if err != nil {
		t.Fatal(err)
	}
	if p.Games != 4 || p.HasINI || p.HasMenu {
		t.Fatalf("plan %+v", p)
	}
	card := t.TempDir()
	if err := runCopy(p, card, func(float64) {}); err != nil {
		t.Fatal(err)
	}
	logf := func(f string, a ...any) { t.Logf(f, a...) }
	renumber(card, logf)
	os.WriteFile(filepath.Join(card, "GDEMU.INI"), []byte(gdemuINI), 0o644)
	if err := installSwirl(card, "", true, logf); err != nil {
		t.Fatal(err)
	}
	c, err := ScanCard(card)
	if err != nil {
		t.Fatal(err)
	}
	if c.MenuType != "SWIRL" || len(c.Games) != 4 || len(c.Warnings) != 0 {
		t.Fatalf("card %s %d %v", c.MenuType, len(c.Games), c.Warnings)
	}
	want := []string{"GAME A", "GAME B", "Zeta Game", "Loose One"}
	for i, g := range c.Games {
		if g.Name != want[i] {
			t.Errorf("slot %s = %q want %q", g.Folder, g.Name, want[i])
		}
	}
	// an empty new card still gets a menu
	empty := t.TempDir()
	if err := installSwirl(empty, "", true, logf); err != nil {
		t.Fatal(err)
	}
	if c, _ := ScanCard(empty); c.MenuType != "SWIRL" {
		t.Fatal("empty card menu", c.MenuType)
	}
	// a second new card made from the first one keeps the menu art and the numbering
	p2, err := planCopy(card)
	if err != nil || p2.Games != 4 || !p2.HasMenu || !p2.HasINI {
		t.Fatalf("plan2 %+v %v", p2, err)
	}
}
