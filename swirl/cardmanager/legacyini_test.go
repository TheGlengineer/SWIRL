package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CM-16: what the old menu's list knew survives the first install and every rebuild after it.
func TestOldMenuListCarried(t *testing.T) {
	root := txnCard(t, "", 3) // 02 GAME A (CDI), 03 GAME B (GDI), 04 GAME C
	ini := strings.Join([]string{
		"[OPENMENU]", "num_items=4", "theme=night", "",
		"[ITEMS]",
		"01.name=openMenu", "01.disc=1/1", "01.vga=1", "01.region=JUE", "01.version=V1.000", "01.date=20200101", "01.product=NEODC_1", "",
		"02.name=Game A, my spelling", "02.disc=1/1", "02.vga=0", "02.region=E", "02.version=V1.000", "02.date=20010101", "02.product=T00002N", "02.folder=Racing", "02.folder_alt1=Arcade", "02.type=game", "",
		"03.name=GAME B", "03.disc=1/1", "03.vga=1", "03.region=JUE", "03.version=V1.000", "03.date=19991231", "03.product=T00003N", "03.shrunk=1", "",
		"04.name=Wrong disc here", "04.disc=1/1", "04.vga=1", "04.region=J", "04.version=V1.000", "04.date=20010101", "04.product=T99999N", "04.custom=x", "",
	}, "\r\n")
	makeMenuIn01With(t, root, "openMenu", map[string][]byte{"OPENMENU.INI": []byte(ini)})
	var lines []string
	logf := func(f string, a ...any) { lines = append(lines, sprintfLine(f, a...)) }
	if err := InstallSwirl(root, "", logf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Read the old menu's OPENMENU.INI: 1 names kept as name.txt, 2 games with their region, VGA or date kept as edits, 3 other settings") {
		t.Fatalf("log %q", lines)
	}
	if readText(filepath.Join(root, "02", "name.txt")) != "Game A, my spelling" || fileExists(filepath.Join(root, "03", "name.txt")) || fileExists(filepath.Join(root, "04", "name.txt")) {
		t.Fatal("name.txt: only the hand edited name is written")
	}
	c, _ := ScanCard(root)
	if g := findGame(c, "02"); g.Name != "Game A, my spelling" || g.Region != "E" || g.VGA || g.Date != "20010101" || !g.Edited {
		t.Fatalf("02: %+v", g)
	}
	if g := findGame(c, "03"); g.Date != "19991231" || !g.Edited || g.Region != "JUE" {
		t.Fatalf("03: %+v", g)
	}
	if g := findGame(c, "04"); g.Name != "GAME C" || g.Region != "JUE" {
		t.Fatalf("04 belongs to a different disc than the list said: %+v", g)
	}
	l := loadLegacyINI(root)
	if l == nil || l.Menu != "openMenu" || l.Header["theme"] != "night" || l.Items["T00002N"]["folder"] != "Racing" || l.Items["T00002N"]["folder_alt1"] != "Arcade" || l.Items["T00003N"]["shrunk"] != "1" || l.Items["T99999N"] != nil {
		t.Fatalf("legacy: %+v", l)
	}
	out, _ := menuINI(root)
	for _, want := range []string{"num_items=4\r\ntheme=night\r\n\r\n[ITEMS]", "02.product=T00002N\r\n02.folder=Racing\r\n02.folder_alt1=Arcade\r\n\r\n03.name", "03.product=T00003N\r\n03.shrunk=1\r\n\r\n04.name", "02.region=E\r\n", "02.vga=0\r\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("INI lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "custom=x") {
		t.Fatal("a key from a slot that held a different disc was carried")
	}
	// after a remove, the extras follow their game to the new slot
	if err := StartRemoveGames(root, []string{"02"}); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	out, _ = menuINI(root)
	if !strings.Contains(out, "02.product=T00003N\r\n02.shrunk=1\r\n") || strings.Contains(out, "folder=Racing") {
		t.Fatalf("after a remove:\n%s", out)
	}
	// a second install over the restored original does not import again or overwrite the owner's edits
	os.WriteFile(filepath.Join(root, "03", "name.txt"), []byte("Game C, renamed later"), 0o644)
	orig := backupsWithPrefix(root, "01_original_openmenu_")[0]
	if err := RestoreBackupForce(root, orig, true, quiet); err != nil {
		t.Fatal(err)
	}
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	if readText(filepath.Join(root, "03", "name.txt")) != "Game C, renamed later" {
		t.Fatal("the owner's later name was overwritten by the old list")
	}
}

// A GDMENU card: LIST.INI names and BLEEM.BIN are carried.
func TestGDMENUListAndBleem(t *testing.T) {
	root := txnCard(t, "", 2)
	list := strings.Join([]string{"[GDMENU]",
		"01.name=GDMENU", "01.disc=1/1", "01.vga=1", "01.region=JUE", "01.version=V0.6.0", "01.date=20160812", "",
		"02.name=Game A (my name)", "02.disc=1/1", "02.vga=1", "02.region=JUE", "02.version=V1.000", "02.date=20010101", "",
		"03.name=GAME B", "03.disc=1/1", "03.vga=1", "03.region=U", "03.version=V1.000", "03.date=20010101", "",
	}, "\r\n")
	makeMenuIn01With(t, root, "GDMENU", map[string][]byte{"LIST.INI": []byte(list), "BLEEM.BIN": []byte("bleem loader bytes")})
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	if readText(filepath.Join(root, "02", "name.txt")) != "Game A (my name)" {
		t.Fatal("the GDMENU name was not kept")
	}
	c, _ := ScanCard(root)
	if g := findGame(c, "03"); g.Region != "U" || !g.Edited {
		t.Fatalf("03: %+v", g)
	}
	m, err := openMenuDisc(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	f, ok := m.files["BLEEM.BIN"]
	if !ok {
		t.Fatal("BLEEM.BIN was not carried from the GDMENU disc")
	}
	b, _ := m.d.readSectors(f.LBA, 1)
	if string(b[:f.Size]) != "bleem loader bytes" {
		t.Fatal("BLEEM.BIN differs")
	}
}

func TestParseMenuINI(t *testing.T) {
	h, s := parseMenuINI("\xef\xbb\xbf[OPENMENU]\r\nnum_items=2\r\nextra = yes\r\n\r\n[ITEMS]\r\n01.name=openMenu\r\n02.Name=Game\r\n02.folder=Racing\r\nbroken line\r\n; comment\r\n")
	if h["num_items"] != "2" || h["extra"] != "yes" || s[1]["name"] != "openMenu" || s[2]["name"] != "Game" || s[2]["folder"] != "Racing" || len(s) != 2 {
		t.Fatalf("header %v slots %v", h, s)
	}
}
