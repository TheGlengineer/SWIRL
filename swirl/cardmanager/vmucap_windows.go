//go:build windows

package main

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

//go:embed assets/swirl-vmucap.exe.gz
var vmucapGz []byte

// vmucapExe unpacks the capture emulator next to the other SWIRL data on this PC.
func vmucapExe() (string, error) {
	dir := filepath.Join(filepath.Dir(dbDir()), "vmucap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	zr, err := gzip.NewReader(bytes.NewReader(vmucapGz))
	if err != nil {
		return "", err
	}
	vmucapBinary, err := io.ReadAll(zr)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "swirl-vmucap.exe")
	if old, err := os.ReadFile(p); err != nil || !bytes.Equal(old, vmucapBinary) {
		if err := os.WriteFile(p, vmucapBinary, 0o755); err != nil {
			return "", err
		}
	}
	// portable mode: keep the emulator's settings and VMU files in its own folder
	os.MkdirAll(filepath.Join(dir, "data"), 0o755)
	if _, err := os.Stat(filepath.Join(dir, "emu.cfg")); err != nil {
		os.WriteFile(filepath.Join(dir, "emu.cfg"), []byte("[config]\nUseReios = yes\n[audio]\nbackend = null\n"), 0o644)
	}
	return p, nil
}

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
