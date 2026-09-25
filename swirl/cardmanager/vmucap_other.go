//go:build !windows

package main

import (
	"errors"
	"os"
	"os/exec"
)

func vmucapExe() (string, error) {
	if p := os.Getenv("SWIRL_VMUCAP_EXE"); p != "" {
		return p, nil
	}
	return "", errors.New("VMU capture is only available on Windows")
}

func hideWindow(cmd *exec.Cmd) {}
