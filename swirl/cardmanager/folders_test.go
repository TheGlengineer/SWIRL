package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// UP-7 and UP-8: four digit folders are seen everywhere, and the gaps GDEMU stops at can be closed.
func TestFourDigitFoldersAndGaps(t *testing.T) {
	root := txnCard(t, "swirl", 2) // 02, 03
	writeTestCDI(t, filepath.Join(root, "1000", "disc.cdi"), "GAME K", "T-01000N")
	writeTestCDI(t, filepath.Join(root, "1001", "disc.cdi"), "GAME L", "T-01001N")
	writeTestCDI(t, filepath.Join(root, "005", "disc.cdi"), "GAME E", "T-00005N")
	os.WriteFile(artPath(root, "1001", "box"), pngBytes(quadImage(32)), 0o644)
	edits := loadEdits(root)
	edits.Games["1001"] = &GameEdit{Product: "T01001N", Region: "J"}
	edits.save()
	c, err := ScanCard(root)
	if err != nil {
		t.Fatal(err)
	}
	if folderMap(root) != "02=T00002N 03=T00003N 005=T00005N 1000=T01000N 1001=T01001N" {
		t.Fatalf("scan: %s", folderMap(root))
	}
	if !c.Gaps || !strings.Contains(strings.Join(c.Warnings, "\n"), "skip a number before 005") {
		t.Fatalf("gaps %v warnings %q", c.Gaps, c.Warnings)
	}
	if g := findGame(c, "1001"); !g.Edited || g.Region != "J" {
		t.Fatalf("edit on a four digit folder: %+v", g)
	}
	h, _ := CheckCard(root)
	if !h.Gaps {
		t.Fatal("health does not offer to close the gaps")
	}
	// a game added now goes after 1001
	src := t.TempDir()
	writeTestCDI(t, filepath.Join(src, "new.cdi"), "GAME N", "T-02000N")
	p, err := planAdd(root, []string{filepath.Join(src, "new.cdi")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.ToSlash(p.Items[0].Dst), "1002/") {
		t.Fatalf("add goes to %s", p.Items[0].Dst)
	}
	if err := StartCloseGaps(root); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	if folderMap(root) != "02=T00002N 03=T00003N 04=T00005N 05=T01000N 06=T01001N" {
		t.Fatalf("after closing the gaps: %s", folderMap(root))
	}
	c, _ = ScanCard(root)
	if c.Gaps || len(c.Warnings) != 0 {
		t.Fatalf("gaps %v warnings %q", c.Gaps, c.Warnings)
	}
	if g := findGame(c, "06"); !g.Edited || g.Region != "J" || !fileExists(artPath(root, "06", "box")) {
		t.Fatalf("the edit and art did not follow the game: %+v", g)
	}
	if msg := iniMatchesFolders(root, c); msg != "" {
		t.Fatal(msg)
	}
	if fileExists(journalPath(root)) {
		t.Fatal("journal left behind")
	}
	// nothing to do the second time
	if err := StartCloseGaps(root); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
}

// Two folders with the same number (02 and 002) are reported and only one is listed.
func TestSameFolderNumberTwice(t *testing.T) {
	root := txnCard(t, "swirl", 2)
	writeTestCDI(t, filepath.Join(root, "002", "disc.cdi"), "GAME X", "T-00099N")
	c, _ := ScanCard(root)
	if len(c.Games) != 2 || !strings.Contains(strings.Join(c.Warnings, "\n"), "Folders 002 and 02 are both number 2") && !strings.Contains(strings.Join(c.Warnings, "\n"), "Folders 02 and 002 are both number 2") {
		t.Fatalf("games %d warnings %q", len(c.Games), c.Warnings)
	}
}

func TestFolderName(t *testing.T) {
	root := t.TempDir()
	for _, n := range []string{"02", "100", "0300", "1000"} {
		os.MkdirAll(filepath.Join(root, n), 0o755)
	}
	for n, want := range map[int]string{2: "02", 100: "100", 300: "0300", 1000: "1000", 7: "07", 1234: "1234"} {
		if got := folderName(root, n); got != want {
			t.Errorf("folderName(%d) = %s, want %s", n, got, want)
		}
	}
	for _, n := range []string{"02", "999", "1000", "9999"} {
		if !folderRe.MatchString(n) {
			t.Errorf("%s is a folder", n)
		}
	}
	for _, n := range []string{"1", "10000", "0x", ""} {
		if folderRe.MatchString(n) {
			t.Errorf("%s is not a folder", n)
		}
	}
}
