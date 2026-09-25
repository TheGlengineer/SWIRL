//go:build !windows && !darwin

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

func vmucapExe() (string, error) {
	if p := os.Getenv("SWIRL_VMUCAP_EXE"); p != "" {
		return p, nil
	}
	return "", errors.New("VMU capture is only available on Windows")
}

func hideWindow(cmd *exec.Cmd) {}

func vmucapDataDir(exe string) string { return filepath.Join(filepath.Dir(exe), "data") }

func vmucapEnv() []string { return nil }

func bringToFront(exe string) {}
