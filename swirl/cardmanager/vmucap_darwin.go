//go:build darwin

package main

// The patched Flycast used for Preview and VMU capture on macOS. The release build puts a zip of
// Flycast.app (built from swirl/vmucap on a Mac, both Apple Silicon and Intel) into
// assets/swirl-vmucap-macos.zip; a local build without it keeps the empty placeholder and those two
// features say they are not included.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

//go:embed assets/swirl-vmucap-macos.zip
var vmucapZip []byte

func vmucapRoot() string { return filepath.Join(appDataDir(), "vmucap") }

// vmucapExe unpacks Flycast.app next to the other SWIRL data once per version.
func vmucapExe() (string, error) {
	if p := os.Getenv("SWIRL_VMUCAP_EXE"); p != "" {
		return p, nil
	}
	if len(vmucapZip) == 0 {
		return "", errors.New("the emulator is not included in this build of SWIRL Card Manager; use a release from GitHub")
	}
	sum := sha256.Sum256(vmucapZip)
	tag := hex.EncodeToString(sum[:6])
	dir := filepath.Join(vmucapRoot(), "app-"+tag)
	exe := filepath.Join(dir, "Flycast.app", "Contents", "MacOS", "Flycast")
	if !fileExists(filepath.Join(dir, ".complete")) {
		os.RemoveAll(dir)
		zr, err := zip.NewReader(bytes.NewReader(vmucapZip), int64(len(vmucapZip)))
		if err != nil {
			return "", err
		}
		if err := extractZipTree(zr, dir); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
		if !fileExists(exe) {
			return "", errors.New("the emulator package is incomplete")
		}
		os.WriteFile(filepath.Join(dir, ".complete"), []byte(tag), 0o644)
		// older unpacked versions
		old, _ := filepath.Glob(filepath.Join(vmucapRoot(), "app-*"))
		for _, o := range old {
			if o != dir {
				os.RemoveAll(o)
			}
		}
	}
	// Flycast keeps its settings in $HOME/.flycast when that folder exists; give it a home of its own
	cfg := filepath.Join(vmucapHome(), ".flycast")
	os.MkdirAll(filepath.Join(cfg, "data"), 0o755)
	if !fileExists(filepath.Join(cfg, "emu.cfg")) {
		os.WriteFile(filepath.Join(cfg, "emu.cfg"), []byte("[config]\nUseReios = yes\n[audio]\nbackend = null\n"), 0o644)
	}
	return exe, nil
}

func vmucapHome() string { return filepath.Join(vmucapRoot(), "home") }

func vmucapDataDir(exe string) string { return filepath.Join(vmucapHome(), ".flycast", "data") }

func vmucapEnv() []string { return []string{"HOME=" + vmucapHome()} }

func hideWindow(cmd *exec.Cmd) {}

// bringToFront: SDL brings the emulator window to the front by itself on macOS (captures, which should
// stay in the background, set SDL_MAC_BACKGROUND_APP).
func bringToFront(exe string) {}
