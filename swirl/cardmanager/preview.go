package main

// Preview in Flycast: builds the exact menu the card would get and opens it in the emulator that ships
// inside SWIRL Card Manager (the same one used for VMU capture), without writing anything to the card.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

var previewCmd *exec.Cmd

func StartPreview(root, dats string) error {
	exe, err := vmucapExe()
	if err != nil {
		return err
	}
	return runJob("Building the preview", "", func() error {
		work, out, _, err := buildMenuImage(root, dats, true, jobLog)
		if work != "" {
			defer os.RemoveAll(work)
		}
		if err != nil {
			return err
		}
		dir := filepath.Join(filepath.Dir(dbDir()), "preview")
		os.RemoveAll(dir)
		if err := copyTree(out, dir); err != nil {
			return err
		}
		if previewCmd != nil && previewCmd.Process != nil {
			previewCmd.Process.Kill()
		}
		gdi := filepath.Join(dir, "disc.gdi")
		if !fileExists(gdi) {
			return errors.New("the preview disc was not built")
		}
		cmd := exec.Command(exe, "-config", "config:UseReios=yes", "-config", "window:width=960", "-config", "window:height=720", gdi)
		cmd.Dir = filepath.Dir(exe)
		if err := cmd.Start(); err != nil {
			return err
		}
		previewCmd = cmd
		go cmd.Wait()
		jobLog("Flycast is open with your menu. Keyboard: arrows move, X is A, C is B, S is X, D is Y, Enter is Start, F and V are the triggers.")
		jobLog("Games will not start from the preview, because the emulator has no GDEMU to switch discs.")
		return nil
	})
}
