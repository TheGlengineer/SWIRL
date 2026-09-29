package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// makeMenuIn01With is makeMenuIn01 with extra files on the menu disc (FOLDRART.DAT marks a VFB menu).
func makeMenuIn01With(t *testing.T, root, ipName string, extra map[string][]byte) {
	t.Helper()
	w := t.TempDir()
	data, low := filepath.Join(w, "data"), filepath.Join(w, "low")
	os.MkdirAll(data, 0o755)
	os.MkdirAll(low, 0o755)
	os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), []byte(strings.Repeat("stock menu binary "+ipName, 300)), 0o644)
	os.WriteFile(filepath.Join(data, "OPENMENU.INI"), []byte("[OPENMENU]\r\nnum_items=1\r\n"), 0o644)
	os.WriteFile(filepath.Join(low, "OPENMENU.INI"), []byte("[OPENMENU]\r\nnum_items=1\r\n"), 0o644)
	for n, b := range extra {
		os.WriteFile(filepath.Join(data, n), b, 0o644)
	}
	os.RemoveAll(filepath.Join(root, "01"))
	os.MkdirAll(filepath.Join(root, "01"), 0o755)
	if err := buildMenuDisc(data, low, filepath.Join(root, "01"), ipSector(ipName, "MENU-001")); err != nil {
		t.Fatal(err)
	}
}

func backupsWithPrefix(root, prefix string) []string {
	var out []string
	for _, b := range listBackups(root) {
		if strings.HasPrefix(b, prefix) {
			out = append(out, b)
		}
	}
	return out
}

// CM-3: whatever menu was on the card before SWIRL is pinned, listed first, and never pruned.
func TestOriginalMenuPinned(t *testing.T) {
	for _, tc := range []struct {
		ip, variant, want string
	}{{"openMenu", "", "01_original_openmenu_"}, {"GDMENU", "", "01_original_gdmenu_"}, {"openMenu", "VFB", "01_original_vfb_"}} {
		t.Run(tc.want, func(t *testing.T) {
			root := txnCard(t, "", 2)
			extra := map[string][]byte{}
			if tc.variant == "VFB" {
				extra["FOLDRART.DAT"] = []byte("virtual folder art")
			}
			makeMenuIn01With(t, root, tc.ip, extra)
			c, _ := ScanCard(root)
			if c.MenuVariant != tc.variant {
				t.Fatalf("variant %q, want %q", c.MenuVariant, tc.variant)
			}
			orig := dirSizes(filepath.Join(root, "01"))
			for i := 0; i < 5; i++ {
				if err := InstallSwirl(root, "", quiet); err != nil {
					t.Fatal(err)
				}
			}
			pinned := backupsWithPrefix(root, tc.want)
			if len(pinned) != 1 || len(backupsWithPrefix(root, originalPrefix)) != 1 {
				t.Fatalf("pinned backups %v (all: %v)", pinned, listBackups(root))
			}
			if !sameSizes(dirSizes(filepath.Join(root, backupDir, pinned[0])), orig) {
				t.Fatal("the pinned backup does not hold the original menu")
			}
			c, _ = ScanCard(root)
			if len(c.BackupList) != 4 || c.BackupList[0].Name != pinned[0] || !c.BackupList[0].Pinned || c.BackupList[0].Kind != "original" {
				t.Fatalf("backup list %+v", c.BackupList)
			}
			if !strings.Contains(c.BackupList[0].Label, "before SWIRL") || (tc.ip == "openMenu" && c.BackupList[0].Menu != "openMenu") {
				t.Fatalf("label %q menu %q", c.BackupList[0].Label, c.BackupList[0].Menu)
			}
			for _, b := range c.BackupList[1:] {
				if b.Pinned || b.Kind != "menu" || b.Menu != "SWIRL" {
					t.Fatalf("automatic backup %+v", b)
				}
			}
		})
	}
}

// A different non SWIRL menu put into 01 by hand later is pinned as well; the restored original is not
// pinned twice.
func TestSecondOriginalPinned(t *testing.T) {
	root := txnCard(t, "openMenu", 2)
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	orig := backupsWithPrefix(root, "01_original_openmenu_")
	if len(orig) != 1 {
		t.Fatal(listBackups(root))
	}
	if err := RestoreBackup(root, orig[0], quiet); err != nil {
		t.Fatal(err)
	}
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	if n := len(backupsWithPrefix(root, originalPrefix)); n != 1 {
		t.Fatalf("%d originals after restoring and reinstalling: %v", n, listBackups(root))
	}
	// the owner tries GDMENU for a while, then comes back
	makeMenuIn01(t, root, "GDMENU")
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	if n := len(backupsWithPrefix(root, "01_original_gdmenu_")); n != 1 {
		t.Fatalf("the GDMENU menu was not pinned: %v", listBackups(root))
	}
	c, _ := ScanCard(root)
	if len(c.BackupList) < 2 || !c.BackupList[0].Pinned || !c.BackupList[1].Pinned || c.BackupList[2].Pinned {
		t.Fatalf("originals are not listed first: %+v", c.BackupList)
	}
}

// Pruning only removes automatic backups of menus SWIRL built, and only past the newest three.
func TestPruneKeepsEverythingButAutoBackups(t *testing.T) {
	root := t.TempDir()
	keep := []string{"01_original_openmenu_20260101_000000", "01_replaced_20260102_000000", "removed_20260103_000000", "removed_junk_20260104_000000", "mine"}
	auto := []string{}
	for i := 0; i < 6; i++ {
		auto = append(auto, fmt.Sprintf("01_202601%02d_120000", 10+i))
	}
	auto = append(auto, "01_20260116_120000_2")
	for _, n := range append(append([]string{}, keep...), auto...) {
		os.MkdirAll(filepath.Join(root, backupDir, n), 0o755)
	}
	pruneMenuBackups(root, 3)
	for _, n := range keep {
		if !fileExists(filepath.Join(root, backupDir, n)) {
			t.Errorf("%s was pruned", n)
		}
	}
	left := backupsWithPrefix(root, "01_20")
	if len(left) != 3 || left[0] != "01_20260116_120000_2" || left[2] != "01_20260114_120000" {
		t.Fatalf("automatic backups left: %v", left)
	}
}

// SWIRL_UI_CARD=dir go test -run TestMakeUICard builds a card with an openMenu original and a few SWIRL
// updates behind it, for trying the Backups page by hand.
func TestMakeUICard(t *testing.T) {
	dir := os.Getenv("SWIRL_UI_CARD")
	if dir == "" {
		t.Skip()
	}
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0o755)
	for i := 0; i < 3; i++ {
		f := fmt.Sprintf("%02d", i+2)
		writeTestCDI(t, filepath.Join(dir, f, "disc.cdi"), fmt.Sprintf("GAME %c", 'A'+i), fmt.Sprintf("T-%05dN", i+2))
	}
	makeMenuIn01(t, dir, "openMenu")
	for i := 0; i < 2; i++ {
		if err := InstallSwirl(dir, "", quiet); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(dir, backupDir, "removed_20260901_120000", "09"), 0o755)
}

// writeTestCDIWith is writeTestCDI with files of the caller's choosing on the disc.
func writeTestCDIWith(t *testing.T, path, title, serial string, files map[string][]byte) {
	t.Helper()
	dir := t.TempDir()
	data := filepath.Join(dir, "d")
	os.MkdirAll(data, 0o755)
	for n, b := range files {
		os.WriteFile(filepath.Join(data, n), b, 0o644)
	}
	iso := filepath.Join(dir, "s.iso")
	if err := buildISO(data, iso, 11702, "T", ipSector(title, serial)); err != nil {
		t.Fatal(err)
	}
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

// CM-4: whatever image is in 01 is named for what it is. A game blocks the install and is never moved.
func TestImageIn01(t *testing.T) {
	t.Run("cdi game", func(t *testing.T) {
		root := txnCard(t, "", 2)
		writeTestCDI(t, filepath.Join(root, "01", "disc.cdi"), "MY CDI GAME", "T-99999N")
		c, err := ScanCard(root)
		if err != nil {
			t.Fatal(err)
		}
		if c.MenuType != "Game" || c.MenuTitle != "MY CDI GAME" || c.MenuImage != "disc.cdi" || c.MenuFormat != "CDI" {
			t.Fatalf("type %q title %q image %q format %q", c.MenuType, c.MenuTitle, c.MenuImage, c.MenuFormat)
		}
		if len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "MY CDI GAME (disc.cdi)") {
			t.Fatalf("warnings %q", c.Warnings)
		}
		err = InstallSwirl(root, "", quiet)
		if err == nil || err.Error() != "folder 01 holds a game, not a menu; nothing was changed" {
			t.Fatalf("install: %v", err)
		}
		if !fileExists(filepath.Join(root, "01", "disc.cdi")) || len(listBackups(root)) != 0 {
			t.Fatalf("the game was moved: %v", listBackups(root))
		}
		h, _ := CheckCard(root)
		found := false
		for _, it := range h.Items {
			if it.Folder == "01" && it.Level == "error" && strings.Contains(it.Message, "MY CDI GAME (disc.cdi)") {
				found = true
			}
		}
		if !found {
			t.Fatalf("health items %+v", h.Items)
		}
	})
	t.Run("mds game", func(t *testing.T) {
		root := txnCard(t, "", 2)
		writeTestCDI(t, filepath.Join(root, "01", "game.mdf"), "MDS GAME", "T-99998N")
		os.WriteFile(filepath.Join(root, "01", "game.mds"), []byte("MEDIA DESCRIPTOR"), 0o644)
		c, _ := ScanCard(root)
		if c.MenuType != "Game" || c.MenuImage != "game.mds" || c.MenuFormat != "MDS" || c.MenuTitle != "MDS GAME" {
			t.Fatalf("type %q title %q image %q format %q", c.MenuType, c.MenuTitle, c.MenuImage, c.MenuFormat)
		}
	})
	t.Run("unreadable image", func(t *testing.T) {
		root := txnCard(t, "", 2)
		os.MkdirAll(filepath.Join(root, "01"), 0o755)
		os.WriteFile(filepath.Join(root, "01", "odd.cdi"), []byte(strings.Repeat("x", 4096)), 0o644)
		c, _ := ScanCard(root)
		if c.MenuType != "Unknown" || c.MenuImage != "odd.cdi" || len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "odd.cdi") {
			t.Fatalf("type %q image %q warnings %q", c.MenuType, c.MenuImage, c.Warnings)
		}
		if err := InstallSwirl(root, "", quiet); err == nil || !strings.Contains(err.Error(), "odd.cdi") {
			t.Fatalf("install: %v", err)
		}
		if !fileExists(filepath.Join(root, "01", "odd.cdi")) {
			t.Fatal("the image was moved")
		}
	})
	t.Run("unknown menu", func(t *testing.T) {
		root := txnCard(t, "", 2)
		makeMenuIn01(t, root, "SOME OTHER MENU")
		c, _ := ScanCard(root)
		if c.MenuType != "Menu" || c.MenuTitle != "SOME OTHER MENU" || len(c.Warnings) != 1 || !strings.Contains(c.Warnings[0], "does not know") {
			t.Fatalf("type %q title %q warnings %q", c.MenuType, c.MenuTitle, c.Warnings)
		}
		for i := 0; i < 5; i++ {
			if err := InstallSwirl(root, "", quiet); err != nil {
				t.Fatal(err)
			}
		}
		if n := backupsWithPrefix(root, "01_original_menu_"); len(n) != 1 {
			t.Fatalf("the unknown menu was not pinned: %v", listBackups(root))
		}
	})
}

// CM-4: a CDI build of openMenu in 01 is a menu. Its art and themes are carried over, it is pinned, and
// the covers page can read it.
func TestCDIMenuIn01(t *testing.T) {
	root := txnCard(t, "", 2)
	box := newDat(131104)
	box.Set("T00002N", []byte("box art for game A"))
	bp := filepath.Join(t.TempDir(), "BOX.DAT")
	if err := box.Write(bp); err != nil {
		t.Fatal(err)
	}
	boxBytes, _ := os.ReadFile(bp)
	writeTestCDIWith(t, filepath.Join(root, "01", "menu.cdi"), "openMenu", "NEODC_1", map[string][]byte{
		"1ST_READ.BIN": []byte(strings.Repeat("stock openMenu ", 200)),
		"OPENMENU.INI": []byte("[OPENMENU]\r\nnum_items=1\r\n"),
		"MARKER.TXT":   []byte("theme file from the CDI menu"),
		"BOX.DAT":      boxBytes,
	})
	c, err := ScanCard(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.MenuType != "openMenu" || c.MenuFormat != "CDI" || c.MenuImage != "menu.cdi" || !c.HasBox {
		t.Fatalf("type %q format %q image %q hasBox %v", c.MenuType, c.MenuFormat, c.MenuImage, c.HasBox)
	}
	if !findGame(c, "02").HasArt {
		t.Fatal("art on the CDI menu was not seen")
	}
	if b, err := GameArt(root, "02", "box"); err == nil && len(b) > 0 {
		t.Log("box art read from the CDI menu")
	}
	for i := 0; i < 5; i++ {
		if err := InstallSwirl(root, "", quiet); err != nil {
			t.Fatal(err)
		}
	}
	c, _ = ScanCard(root)
	if c.MenuType != "SWIRL" || c.MenuFormat != "GDI" {
		t.Fatalf("after install: type %q format %q", c.MenuType, c.MenuFormat)
	}
	m, err := openMenuDisc(root)
	if err != nil {
		t.Fatal(err)
	}
	_, marker := m.files["MARKER.TXT"]
	_, hasBox := m.files["BOX.DAT"]
	m.Close()
	if !marker || !hasBox {
		t.Fatalf("files from the CDI menu were not carried over: marker %v box %v", marker, hasBox)
	}
	pinned := backupsWithPrefix(root, "01_original_openmenu_")
	if len(pinned) != 1 || !fileExists(filepath.Join(root, backupDir, pinned[0], "menu.cdi")) {
		t.Fatalf("the CDI menu was not pinned: %v", listBackups(root))
	}
	c, _ = ScanCard(root)
	if c.BackupList[0].Name != pinned[0] || c.BackupList[0].Menu != "openMenu" {
		t.Fatalf("backup list %+v", c.BackupList)
	}
}

// An automatic backup that holds a game (made by an older Card Manager) is never pruned.
func TestPruneSkipsBackupsHoldingGames(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		writeTestCDI(t, filepath.Join(root, backupDir, fmt.Sprintf("01_202601%02d_120000", 10+i), "disc.cdi"), "OLD GAME", "T-00001N")
	}
	os.MkdirAll(filepath.Join(root, backupDir, "01_20260120_120000"), 0o755)
	pruneMenuBackups(root, 1)
	if n := len(listBackups(root)); n != 6 {
		t.Fatalf("%d backups left: %v", n, listBackups(root))
	}
}

// UP-2: every backup carries a manifest; a backup with a missing file is refused; a backup whose game
// list no longer matches the folders is refused unless forced.
func TestBackupManifest(t *testing.T) {
	root := txnCard(t, "openMenu", 3)
	unlock, _ := lockCard(root, "Update SWIRL")
	err := InstallSwirl(root, "", quiet)
	unlock()
	if err != nil {
		t.Fatal(err)
	}
	orig := backupsWithPrefix(root, "01_original_openmenu_")[0]
	m := readBackupManifest(filepath.Join(root, backupDir, orig))
	if m == nil {
		t.Fatal("no manifest")
	}
	if m.Menu != "openMenu" || m.Reason != "Update SWIRL" || m.MenuFormat != "GDI" || len(m.Slots) != 3 || m.Slots["02"].Product != "T00002N" || m.Files["track03.iso"] == 0 || m.CardManager != version {
		t.Fatalf("manifest %+v", m)
	}
	c, _ := ScanCard(root)
	b := c.BackupList[0]
	if b.Reason != "Update SWIRL" || b.Games != 3 || b.When == "" || !strings.Contains(b.Label, "3 games") || b.Note != "" {
		t.Fatalf("backup item %+v", b)
	}
	if !fileExists(filepath.Join(root, backupDir, orig+".json")) {
		t.Fatal("the manifest is not next to the backup")
	}
	if err := RestoreBackup(root, orig, quiet); err != nil {
		t.Fatal(err)
	}
	c, _ = ScanCard(root)
	rep := c.BackupList[1]
	if rep.Kind != "replaced" || rep.Menu != "SWIRL" || !strings.HasPrefix(rep.Reason, "Restore of 01_original_") || rep.Release == "" {
		t.Fatalf("replaced item %+v", rep)
	}
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}

	// the folders change: 03 removed and the rest renumbered
	if err := StartRemoveGames(root, []string{"03"}); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	err = RestoreBackup(root, orig, quiet)
	var mm *restoreMismatch
	if !errors.As(err, &mm) {
		t.Fatalf("restore after a renumber: %v", err)
	}
	want := []string{"slot 03 was GAME B, is now GAME C", "slot 04 was GAME C, is now empty"}
	if strings.Join(mm.Lines, "|") != strings.Join(want, "|") {
		t.Fatalf("mismatches %q", mm.Lines)
	}
	if !fileExists(filepath.Join(root, "01", "track05.iso")) || len(backupsWithPrefix(root, "01_replaced_")) != 1 {
		t.Fatal("the refused restore changed the card")
	}
	if err := RestoreBackupForce(root, orig, true, quiet); err != nil {
		t.Fatal(err)
	}
	c, _ = ScanCard(root)
	if c.MenuType != "openMenu" {
		t.Fatalf("forced restore: %s", c.MenuType)
	}

	// a backup that lost a file is refused with a message, and the page says so
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	newest := listBackups(root)
	var auto string
	for _, n := range newest {
		if autoBackupRe.MatchString(n) {
			auto = n
			break
		}
	}
	os.Remove(filepath.Join(root, backupDir, auto, "track05.iso"))
	err = RestoreBackup(root, auto, quiet)
	if err == nil || !strings.Contains(err.Error(), "incomplete: track05.iso is missing") {
		t.Fatalf("restore of an incomplete backup: %v", err)
	}
	c, _ = ScanCard(root)
	found := false
	for _, b := range c.BackupList {
		if b.Name == auto {
			found = strings.Contains(b.Note, "track05.iso is missing")
		}
	}
	if !found {
		t.Fatalf("no note on the incomplete backup: %+v", c.BackupList)
	}
	// a short file too
	os.WriteFile(filepath.Join(root, backupDir, auto, "track05.iso"), []byte("short"), 0o644)
	if err := RestoreBackup(root, auto, quiet); err == nil || !strings.Contains(err.Error(), "track05.iso is 5 bytes") {
		t.Fatalf("restore of a short backup: %v", err)
	}
}

// L13: two installs in one second make two backups, each with its own manifest.
func TestBackupsInOneSecond(t *testing.T) {
	root := txnCard(t, "openMenu", 2)
	time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
	for i := 0; i < 3; i++ {
		if err := InstallSwirl(root, "", quiet); err != nil {
			t.Fatal(err)
		}
	}
	bk := listBackups(root)
	if len(bk) != 3 {
		t.Fatalf("backups %v", bk)
	}
	for _, b := range bk {
		if readBackupManifest(filepath.Join(root, backupDir, b)) == nil {
			t.Fatalf("%s has no manifest", b)
		}
	}
	// pruning takes the manifest with the folder
	for i := 0; i < 3; i++ {
		if err := InstallSwirl(root, "", quiet); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(root, backupDir))
	dirs, files := 0, 0
	for _, e := range entries {
		if e.IsDir() {
			dirs++
		} else {
			files++
		}
	}
	if dirs != 4 || files != 4 {
		t.Fatalf("%d backup folders and %d manifests", dirs, files)
	}
}

// SWIRL_UI_CARD2=dir go test -run TestMakeUICard2: the card above after a game was removed (the original's
// list no longer matches) and with one automatic backup missing a track.
func TestMakeUICard2(t *testing.T) {
	dir := os.Getenv("SWIRL_UI_CARD2")
	if dir == "" {
		t.Skip()
	}
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0o755)
	for i := 0; i < 3; i++ {
		f := fmt.Sprintf("%02d", i+2)
		writeTestCDI(t, filepath.Join(dir, f, "disc.cdi"), fmt.Sprintf("GAME %c", 'A'+i), fmt.Sprintf("T-%05dN", i+2))
	}
	makeMenuIn01(t, dir, "openMenu")
	for i := 0; i < 2; i++ {
		if err := InstallSwirl(dir, "", quiet); err != nil {
			t.Fatal(err)
		}
	}
	if err := StartRemoveGames(dir, []string{"02"}); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	for _, b := range listBackups(dir) {
		if autoBackupRe.MatchString(b) {
			os.Remove(filepath.Join(dir, backupDir, b, "track03.iso"))
			break
		}
	}
}
