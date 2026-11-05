package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scanTestEnv(t *testing.T) string {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", os.Getenv("LOCALAPPDATA"))
	t.Setenv("HOME", os.Getenv("LOCALAPPDATA"))
	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "FIRST GAME", "T-00001N")
	writeTestCDI(t, filepath.Join(card, "03", "disc.cdi"), "SECOND GAME", "T-00002N")
	return card
}

// A scan tells its story: the steps, one line per folder with its time, the result, and the same on disk.
func TestScanLogLines(t *testing.T) {
	card := scanTestEnv(t)
	c, err := ScanCard(card)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Games) != 2 {
		t.Fatalf("%d games", len(c.Games))
	}
	st := ScanStatus()
	if st.Running || !st.Finished || st.Error != "" || st.Done != 2 || st.Total != 2 || st.Pct != 1 || st.Root != card {
		t.Fatalf("status %+v", st)
	}
	text := strings.Join(st.Log, "\n")
	for _, want := range []string{"Reading the menu in folder 01", "01  no menu disc", "Reading 2 game folders", "02  FIRST GAME  CDI  ", "03  SECOND GAME  CDI  ", "Done in "} {
		if !strings.Contains(text, want) {
			t.Fatalf("log lacks %q:\n%s", want, text)
		}
	}
	if len(st.Slowest) != 2 || st.Slowest[0].Ms < st.Slowest[1].Ms {
		t.Fatalf("slowest %+v", st.Slowest)
	}
	b, err := os.ReadFile(scanLogPath())
	if err != nil {
		t.Fatal(err)
	}
	disk := string(b)
	if !strings.HasPrefix(disk, "SWIRL Card Manager "+version+" scan log") || !strings.Contains(disk, "card: "+card) || !strings.Contains(disk, "03  SECOND GAME") || !strings.Contains(disk, "Done in ") {
		t.Fatalf("scan.log:\n%s", disk)
	}
	rep := ScanReport(card)
	for _, want := range []string{"SWIRL Card Manager " + version, "Card: " + card, "Scan finished in ", "2 folders", "Slowest folders:", "Scan log (" + scanLogPath() + ")", "  03  SECOND GAME"} {
		if !strings.Contains(rep, want) {
			t.Fatalf("report lacks %q:\n%s", want, rep)
		}
	}
	path, err := SaveScanReport(card)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != rep {
		t.Fatalf("saved report differs (%v)", err)
	}
}

// A panic while a folder is read comes back as an error that names the folder, with the log kept, instead
// of a window that sits on "Reading".
func TestScanPanicRecovered(t *testing.T) {
	card := scanTestEnv(t)
	scanTestHook = func(folder string) {
		if folder == "03" {
			var m map[string]int
			m["x"] = 1 // nil map write, the kind of slip that would otherwise take the server down
		}
	}
	defer func() { scanTestHook = nil }()
	c, err := ScanCard(card)
	if err == nil || c != nil {
		t.Fatalf("no error (%v)", c)
	}
	if !strings.Contains(err.Error(), "unexpected problem") || !strings.Contains(err.Error(), "folder 03") || !strings.Contains(err.Error(), "Report a problem") {
		t.Fatalf("error %q", err)
	}
	st := ScanStatus()
	text := strings.Join(st.Log, "\n")
	if st.Running || !st.Finished || st.Error == "" || st.Stage != "Stopped" || !strings.Contains(text, "02  FIRST GAME") || !strings.Contains(text, "assignment to entry in nil map") || !strings.Contains(text, "Stopped after") {
		t.Fatalf("status %+v", st)
	}
	// the next scan is a new story
	scanTestHook = nil
	if _, err := ScanCard(card); err != nil {
		t.Fatal(err)
	}
	if st := ScanStatus(); st.Error != "" || st.Done != 2 {
		t.Fatalf("status after %+v", st)
	}
	if rep := ScanReport(card); strings.Contains(rep, "unexpected problem") {
		t.Fatal("the report still carries the old scan")
	}
}

// A root that is not a folder fails at once, with the reason and the step it failed at in the log.
func TestScanBadRoot(t *testing.T) {
	scanTestEnv(t)
	missing := filepath.Join(t.TempDir(), "gone")
	if _, err := ScanCard(missing); err == nil || !strings.Contains(err.Error(), "is not a folder") {
		t.Fatalf("error %v", err)
	}
	st := ScanStatus()
	if st.Error == "" || !strings.Contains(strings.Join(st.Log, "\n"), "Checking the card's folder") {
		t.Fatalf("status %+v", st)
	}
	if rep := ScanReport(missing); !strings.Contains(rep, "Scan stopped after") {
		t.Fatalf("report:\n%s", rep)
	}
}
