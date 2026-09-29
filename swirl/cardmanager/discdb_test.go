package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vfbDB writes a DISCDB.JSON in the Virtual Folder Bundle's shape for the named folders (folder -> serial,
// name), with one extra key SWIRL does not know on every entry.
func vfbDB(t *testing.T, root string, entries map[string][2]string) {
	t.Helper()
	items := map[string]any{}
	for f, e := range entries {
		items[f] = map[string]any{"name": e[1], "serial": e[0], "type": "game", "disc": "1/1", "vga": true, "region": "U",
			"version": "V1.000", "date": "20000101", "folder": "Racing", "altFolders": []string{"Arcade"}, "shrunk": false, "bundleOnly": "kept"}
	}
	b, _ := json.MarshalIndent(map[string]any{"version": 1, "items": items, "generator": "vfb test"}, "", "  ")
	if err := os.WriteFile(filepath.Join(root, discDBName), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func readDB(t *testing.T, root string) map[string]discDBEntry {
	t.Helper()
	items, _, err := readDiscDB(filepath.Join(root, discDBName))
	if err != nil {
		t.Fatal(err)
	}
	return items
}

// UP-1: DISCDB.JSON follows the games through a remove and renumber, a stale entry is corrected, a new
// game gets an entry, and keys SWIRL does not know survive.
func TestDiscDBKeptInStep(t *testing.T) {
	root := txnCard(t, "openMenu", 4) // 02 A, 03 B, 04 C, 05 D
	// 04 is stale (the audit's slot 17: the file says one game, the folder holds another); 05 has no entry
	vfbDB(t, root, map[string][2]string{"02": {"T00002N", "GAME A"}, "03": {"T-00003N", "Game B renamed"}, "04": {"T1217N", "DINO CRISIS"}})
	c, _ := ScanCard(root)
	probs := discDBProblems(root, c)
	if len(probs) != 3 || !strings.Contains(probs[0], "has no entry for folder 05") || !strings.Contains(probs[1], "says folder 04 is DINO CRISIS (T1217N) but it holds GAME C (T00004N)") {
		t.Fatalf("problems %q", probs)
	}
	h, _ := CheckCard(root)
	found := false
	for _, it := range h.Items {
		if strings.Contains(it.Message, "DINO CRISIS") {
			found = true
		}
	}
	if !found {
		t.Fatalf("health does not report the stale entry: %+v", h.Items)
	}
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	db := readDB(t, root)
	if len(db) != 4 || db["04"].str("serial") != "T00004N" || db["04"].str("name") != "GAME C" || db["05"].str("serial") != "T00005N" || db["05"].str("type") != "game" {
		t.Fatalf("after install: %v", db)
	}
	if db["03"].str("name") != "Game B renamed" || db["03"].str("bundleOnly") != "kept" || db["03"].str("folder") != "Racing" {
		t.Fatalf("entry 03 was not kept as it was: %v", db["03"])
	}
	if len(discDBProblems(root, c)) != 0 {
		t.Fatalf("still wrong: %q", discDBProblems(root, c))
	}
	// remove 02: B, C, D move to 02, 03, 04 and their entries move with them
	if err := StartRemoveGames(root, []string{"02"}); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	db = readDB(t, root)
	if len(db) != 3 || db["02"].str("serial") != "T-00003N" || db["02"].str("name") != "Game B renamed" || db["02"].str("bundleOnly") != "kept" || db["03"].str("serial") != "T00004N" || db["04"].str("serial") != "T00005N" {
		t.Fatalf("after remove: %v", db)
	}
	// a name typed in SWIRL follows into the entry; the top level keys are still there
	os.WriteFile(filepath.Join(root, "03", "name.txt"), []byte("Game C, my name"), 0o644)
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	db = readDB(t, root)
	if db["03"].str("name") != "Game C, my name" {
		t.Fatalf("name not followed: %v", db["03"])
	}
	b, _ := os.ReadFile(filepath.Join(root, discDBName))
	var top map[string]any
	json.Unmarshal(b, &top)
	if top["generator"] != "vfb test" || top["version"] != float64(1) {
		t.Fatalf("top level keys: %v", top)
	}
	// nothing to change: the file is left alone
	st1, _ := os.Stat(filepath.Join(root, discDBName))
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	st2, _ := os.Stat(filepath.Join(root, discDBName))
	if !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("DISCDB.JSON was rewritten with nothing to change")
	}
}

// Two copies of one game keep their own entries through a renumber.
func TestDiscDBDuplicates(t *testing.T) {
	root := txnCard(t, "openMenu", 3)
	writeTestCDI(t, filepath.Join(root, "05", "disc.cdi"), "GAME A", "T-00002N") // a second copy of 02
	vfbDB(t, root, map[string][2]string{"02": {"T00002N", "GAME A first"}, "03": {"T00003N", "GAME B"}, "04": {"T00004N", "GAME C"}, "05": {"T00002N", "GAME A second"}})
	if err := StartRemoveGames(root, []string{"03"}); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	db := readDB(t, root)
	if db["02"].str("name") != "GAME A first" || db["03"].str("name") != "GAME C" || db["04"].str("name") != "GAME A second" {
		t.Fatalf("%v", db)
	}
}

// A DISCDB.JSON SWIRL cannot read is moved aside with a note, never left stale.
func TestDiscDBSetAside(t *testing.T) {
	for name, body := range map[string]string{"version 2": `{"version":2,"items":{}}`, "not json": `{"version":1,"items":`} {
		t.Run(name, func(t *testing.T) {
			root := txnCard(t, "openMenu", 2)
			os.WriteFile(filepath.Join(root, discDBName), []byte(body), 0o644)
			c, _ := ScanCard(root)
			if p := discDBProblems(root, c); len(p) != 1 || !strings.Contains(p[0], "cannot be read") {
				t.Fatalf("problems %q", p)
			}
			var lines []string
			if err := InstallSwirl(root, "", func(f string, a ...any) { lines = append(lines, sprintfLine(f, a...)) }); err != nil {
				t.Fatal(err)
			}
			if fileExists(filepath.Join(root, discDBName)) {
				t.Fatal("the unreadable file was left on the card")
			}
			entries, _ := os.ReadDir(filepath.Join(root, backupDir))
			var moved, note string
			for _, e := range entries {
				switch {
				case strings.HasPrefix(e.Name(), "DISCDB_") && strings.HasSuffix(e.Name(), ".JSON"):
					moved = e.Name()
				case strings.HasPrefix(e.Name(), "DISCDB_") && strings.HasSuffix(e.Name(), ".TXT"):
					note = e.Name()
				}
			}
			if moved == "" || note == "" {
				t.Fatalf("moved %q note %q", moved, note)
			}
			if b, _ := os.ReadFile(filepath.Join(root, backupDir, moved)); string(b) != body {
				t.Fatal("the moved file was changed")
			}
			if b, _ := os.ReadFile(filepath.Join(root, backupDir, note)); !strings.Contains(string(b), "name.txt and serial.txt") {
				t.Fatalf("note: %s", b)
			}
			if !strings.Contains(strings.Join(lines, "\n"), "moved to SWIRL_BACKUP") {
				t.Fatalf("log %q", lines)
			}
		})
	}
}

func sprintfLine(f string, a ...any) string { return strings.TrimSpace(fmt.Sprintf(f, a...)) }
