package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
				if b.Pinned || b.Kind != "menu" || b.Menu != "openMenu" {
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
