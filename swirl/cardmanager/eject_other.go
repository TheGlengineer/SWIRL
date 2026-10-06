//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// ejectDrive unmounts the card on a Mac or Linux PC so it is safe to pull.
func ejectDrive(root string) error {
	if _, err := os.Stat(root); err != nil {
		return errors.New("the card is not in the reader")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("diskutil", "unmount", root)
	default:
		if _, err := exec.LookPath("udisksctl"); err == nil {
			// udisksctl wants the device; umount by mount point works for the desktop's own mounts
			cmd = exec.Command("umount", root)
		} else {
			cmd = exec.Command("umount", root)
		}
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}
