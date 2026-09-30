//go:build linux

package main

// The patched Flycast used for Preview and VMU capture on Linux. The release build puts a gzipped
// flycast binary (built from swirl/vmucap) into assets/swirl-vmucap-linux.gz; a local checkout keeps
// the empty placeholder and those two features say they are not included, exactly like the macOS zip.

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed assets/swirl-vmucap-linux.gz
var vmucapGz []byte

// vmucapExe unpacks the capture emulator next to the other SWIRL data once.
func vmucapExe() (string, error) {
	if p := os.Getenv("SWIRL_VMUCAP_EXE"); p != "" {
		return p, nil
	}
	if len(vmucapGz) == 0 {
		return "", errors.New("the emulator is not included in this build of SWIRL Card Manager; use a release from GitHub")
	}
	dir := filepath.Join(filepath.Dir(dbDir()), "vmucap")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	zr, err := gzip.NewReader(bytes.NewReader(vmucapGz))
	if err != nil {
		return "", err
	}
	bin, err := io.ReadAll(zr)
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "swirl-vmucap")
	if old, err := os.ReadFile(p); err != nil || !bytes.Equal(old, bin) {
		// Write to a temp file in the same directory and rename over p, so a
		// still-running older copy is never overwritten in place (which would fail
		// with ETXTBSY on Linux).
		tmp, err := os.CreateTemp(dir, "swirl-vmucap-*.tmp")
		if err != nil {
			return "", err
		}
		tmpName := tmp.Name()
		if _, err := tmp.Write(bin); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return "", err
		}
		if err := tmp.Chmod(0o755); err != nil {
			tmp.Close()
			os.Remove(tmpName)
			return "", err
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmpName)
			return "", err
		}
		if err := os.Rename(tmpName, p); err != nil {
			os.Remove(tmpName)
			return "", err
		}
	}
	// portable mode: Flycast keeps its settings and VMU files in its own home, not ~/.flycast
	if err := os.MkdirAll(filepath.Join(vmucapHome(), ".flycast", "data"), 0o755); err != nil {
		return "", err
	}
	cfg := filepath.Join(vmucapHome(), ".flycast", "emu.cfg")
	if _, err := os.Stat(cfg); err != nil {
		if err := os.WriteFile(cfg, []byte("[config]\nUseReios = yes\n[audio]\nbackend = null\n"), 0o644); err != nil {
			return "", err
		}
	}
	return p, nil
}

func vmucapHome() string { return filepath.Join(filepath.Dir(dbDir()), "vmucap", "home") }

func vmucapDataDir(exe string) string { return filepath.Join(vmucapHome(), ".flycast", "data") }

func vmucapEnv() []string { return []string{"HOME=" + vmucapHome()} }

func hideWindow(cmd *exec.Cmd) {}

// bringToFront: SDL raises the emulator window on its own during "Capture by playing"; there is
// nothing to do for the hidden automatic runs.
func bringToFront(exe string) {}
