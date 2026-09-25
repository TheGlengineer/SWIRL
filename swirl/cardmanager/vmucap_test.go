package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestRawToSwirlRoundTrip(t *testing.T) {
	raw := make([]byte, 192)
	raw[5] = 1 // first byte read for row 0 holds x=0..7, LSB = x 0
	b := rawToSwirl(raw)
	if b[31*6] != 0x80 {
		t.Fatalf("pixel (0,0) of the raw block should land at bottom-left, got %x", b[31*6])
	}
}

// Needs the Linux build of the patched emulator and xvfb: SWIRL_FLYCAST=/path/flycast
func TestCaptureSwirlMenu(t *testing.T) {
	fc := os.Getenv("SWIRL_FLYCAST")
	if fc == "" {
		t.Skip("set SWIRL_FLYCAST")
	}
	captureCommand = func(ctx context.Context, disc, logFile string) (*exec.Cmd, error) {
		return exec.CommandContext(ctx, "xvfb-run", "-a", fc, "-config", "config:UseReios=yes", "-config", "audio:backend=null", disc), nil
	}
	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "CHUCHU ROCKET", "MK-51049")
	if err := installSwirl(card, "", false, t.Logf); err != nil {
		t.Fatal(err)
	}
	shots, err := captureGame(findDisc(filepath.Join(card, "01")))
	if err != nil || len(shots) == 0 {
		t.Fatal(shots, err)
	}
	for i, s := range shots {
		t.Logf("shot %d: %.1fs from %.1fs", i, s.Seconds, s.First)
		os.WriteFile(filepath.Join(os.TempDir(), "swirl_shot_"+string(rune('0'+i))+".png"), vmuPNG(s.Bits), 0o644)
	}
	// and the whole job, storing the pick so the menu build puts it in VMU.DAT
	os.Setenv("SWIRL_VMUCAP_EXE", fc)
	// point the game folder at the menu disc so there is something that draws
	os.Rename(filepath.Join(card, "02", "disc.cdi"), filepath.Join(card, "disc.cdi.bak"))
	for _, n := range []string{"disc.gdi", "track01.iso", "track02.raw", "track03.iso", "track04.raw", "track05.iso"} {
		b, _ := os.ReadFile(filepath.Join(card, "01", n))
		os.WriteFile(filepath.Join(card, "02", n), b, 0o644)
	}
	if err := StartVMUCapture(card, false); err != nil {
		t.Fatal(err)
	}
	j := waitJobLong(t)
	t.Log(j.Log)
	caps := loadCaptures(card)
	if len(caps) != 1 || caps[0].Chosen != 0 || !fileExists(artPath(card, "02", "vmu")) {
		t.Fatalf("captures %+v", caps)
	}
	if err := ChooseVMU(card, "02", len(caps[0].Shots)-1); err != nil {
		t.Fatal(err)
	}
}

func TestManualCapture(t *testing.T) {
	fc := os.Getenv("SWIRL_FLYCAST")
	if fc == "" {
		t.Skip("set SWIRL_FLYCAST")
	}
	manualCommand = func(disc string) (*exec.Cmd, error) {
		// stands in for the person closing the window after 20 seconds
		return exec.Command("timeout", "20", "xvfb-run", "-a", fc, "-config", "config:UseReios=yes", "-config", "audio:backend=null", disc), nil
	}
	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "X", "MK-51049")
	if err := installSwirl(card, "", false, t.Logf); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"disc.gdi", "track01.iso", "track02.raw", "track03.iso", "track04.raw", "track05.iso"} {
		b, _ := os.ReadFile(filepath.Join(card, "01", n))
		os.WriteFile(filepath.Join(card, "02", n), b, 0o644)
	}
	os.Remove(filepath.Join(card, "02", "disc.cdi"))
	if err := StartManualCapture(card, "02"); err != nil {
		t.Fatal(err)
	}
	j := waitJobLong(t)
	t.Log(j.Log)
	if c := loadCaptures(card); len(c) != 1 || len(c[0].Shots) == 0 {
		t.Fatalf("%+v", c)
	}
}
