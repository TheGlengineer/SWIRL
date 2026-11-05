//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// One folder the card never answers for (a disc image that blocks on open) is skipped after the time
// limit with a warning naming it; the other games still come through, and the log says which was skipped.
func TestScanFolderTimeout(t *testing.T) {
	card := scanTestEnv(t)
	stuck := filepath.Join(card, "04")
	os.MkdirAll(stuck, 0o755)
	fifo := filepath.Join(stuck, "disc.cdi")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		t.Skip("no fifo here:", err)
	}
	old := scanFolderLimit
	scanFolderLimit = 300 * time.Millisecond
	defer func() { scanFolderLimit = old }()
	// the read blocks on open until a writer comes; it is let go at the end so the test leaves no goroutine
	defer func() {
		if w, err := os.OpenFile(fifo, os.O_WRONLY, 0); err == nil {
			w.Close()
		}
	}()
	began := time.Now()
	c, err := ScanCard(card)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(began) > 5*time.Second {
		t.Fatal("the scan waited for the stuck folder")
	}
	if len(c.Games) != 3 {
		t.Fatalf("%d games", len(c.Games))
	}
	g := c.Games[2]
	if g.Folder != "04" || !strings.Contains(g.Error, "took more than") || g.Name != "Folder 04" {
		t.Fatalf("stuck game %+v", g)
	}
	if c.Games[0].Error != "" || c.Games[1].Error != "" {
		t.Fatalf("the other games were touched: %+v", c.Games[:2])
	}
	found := false
	for _, w := range c.Warnings {
		if strings.Contains(w, "Folder 04 took more than") && strings.Contains(w, "skipped this time") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no warning names the folder: %v", c.Warnings)
	}
	st := ScanStatus()
	if len(st.Skipped) != 1 || st.Skipped[0] != "04" || !strings.Contains(strings.Join(st.Log, "\n"), "04  Folder 04  no disc image") || !strings.Contains(strings.Join(st.Log, "\n"), "SKIPPED") {
		t.Fatalf("status %+v", st)
	}
	if rep := ScanReport(card); !strings.Contains(rep, "Folders skipped for taking more than") || !strings.Contains(rep, ": 04") {
		t.Fatalf("report:\n%s", rep)
	}
}
