//go:build linux

package main

// SWIRL Card Manager on Linux: a single static binary. It has no window of its own; its window is
// Chromium/Chrome/Edge/Brave/Vivaldi in app mode (or a tab in the default browser). Installing copies
// the binary into ~/.local/share/swirl-card-manager and adds a .desktop entry, all without root.
// Updating gunzips the downloaded binary over the installed one.

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const appName = "SWIRL Card Manager"

var windowKind = "browser"

// installDir is where the binary lives: $XDG_DATA_HOME/swirl-card-manager, else ~/.local/share.
func installDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "swirl-card-manager")
}

func installedExe() string { return filepath.Join(installDir(), "SWIRL-Card-Manager") }

func selfExe() string {
	p, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

func runningInstalled() bool { return samePath(selfExe(), installedExe()) }

func installedVersion() string {
	b, err := os.ReadFile(filepath.Join(installDir(), "version.txt"))
	if err != nil || !fileExists(installedExe()) {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func getAppInfo() AppInfo {
	iv := installedVersion()
	return AppInfo{
		Version: version, Platform: "linux", Installed: iv != "", InstalledVersion: iv,
		RunningInstalled: runningInstalled(), CanInstall: true, Window: windowKind, InstallDir: installDir(),
		DataBytes: dirBytes(appDataDir()),
	}
}

// ---------- the window ----------

// findBrowser returns the path to a Chromium-based browser, if one is installed.
func findBrowser() string {
	names := []string{
		"google-chrome-stable", "google-chrome", "chromium", "chromium-browser",
		"microsoft-edge", "microsoft-edge-stable", "brave-browser", "brave", "vivaldi-stable", "vivaldi",
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	cands := []string{"/usr/bin", "/usr/local/bin", "/snap/bin", "/opt/google/chrome"}
	for _, c := range cands {
		matches, _ := filepath.Glob(filepath.Join(c, "*chrom*"))
		for _, m := range matches {
			if fileExists(m) {
				return m
			}
		}
	}
	return ""
}

// openWindow shows the app in its own Chromium window with no address bar, using a profile of its own.
// Without a Chromium browser it opens a tab in the default browser.
func openWindow(url string) {
	exe := findBrowser()
	if exe == "" {
		windowKind = "browser"
		openBrowser(url)
		return
	}
	args := []string{
		"--app=" + url,
		"--user-data-dir=" + filepath.Join(appDataDir(), "window"),
		"--window-size=1360,880",
		"--no-first-run", "--no-default-browser-check", "--disable-sync", "--disable-extensions",
		"--hide-crash-restore-bubble",
	}
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		windowKind = "browser"
		openBrowser(url)
		return
	}
	windowKind = "app"
	go cmd.Wait()
}

// ---------- install, update, uninstall ----------

func writable(dir string) bool {
	f, err := os.CreateTemp(dir, ".swirl-write-test")
	if err != nil {
		return false
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	return true
}

// copyExe copies src over dst, swapping it in only once the copy is complete.
func copyExe(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	os.Chmod(tmp, 0o755)
	old := dst + ".old"
	os.Remove(old)
	if fileExists(dst) {
		if err := os.Rename(dst, old); err != nil {
			os.Remove(tmp)
			return errors.New("the installed copy is in use; close it and try again")
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Rename(old, dst)
		return err
	}
	os.Remove(old)
	return nil
}

func desktopFile() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "applications", "swirl-card-manager.desktop")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "applications", "swirl-card-manager.desktop")
}

// writeDesktop installs (or removes) the .desktop entry. The icon is unpacked from the embedded web
// assets into the install dir.
func writeDesktop(exe string) error {
	icon := filepath.Join(installDir(), "icon-256.png")
	os.MkdirAll(installDir(), 0o755)
	if b, err := webFS.ReadFile("web/icon-256.png"); err == nil {
		os.WriteFile(icon, b, 0o644)
	}
	d := desktopFile()
	os.MkdirAll(filepath.Dir(d), 0o755)
	content := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=SWIRL Card Manager
Comment=Set up GDEMU SD cards with the SWIRL menu
Exec=%s
Icon=%s
Terminal=false
Categories=Utility;
`, shellQuote(exe), icon)
	return os.WriteFile(d, []byte(content), 0o644)
}

// shellQuote quotes a single path for use in a .desktop Exec= line.
func shellQuote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }

// InstallApp copies this binary into installDir and adds the .desktop entry. No root needed.
func InstallApp(desktop bool) error {
	if runningInstalled() {
		return errors.New("this is already the installed copy")
	}
	dir := installDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	exe := installedExe()
	if err := copyExe(selfExe(), exe); err != nil {
		return err
	}
	os.WriteFile(filepath.Join(dir, "version.txt"), []byte(version), 0o644)
	if err := writeDesktop(exe); err != nil {
		return fmt.Errorf("installed, but the launcher entry could not be made (%v); it can be started from %s", err, exe)
	}
	return nil
}

// SwitchToInstalled starts the installed copy once this one has quit.
func SwitchToInstalled() error {
	exe := installedExe()
	if !fileExists(exe) {
		return errors.New("SWIRL Card Manager is not installed")
	}
	cmd := exec.Command(exe, "-wait-pid", strconv.Itoa(os.Getpid()))
	cmd.Dir = installDir()
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		quitNow()
	}()
	return nil
}

// Uninstall removes the .desktop entry and the installed binary. With quiet it also deletes the data
// directory (art database, emulator files and settings). Without a desktop dialog, a non-quiet
// uninstall keeps the data for a later install.
func Uninstall(quiet bool) int {
	removeData := quiet
	if ri := otherInstance(); ri != nil {
		askToQuit(ri)
	}
	os.Remove(desktopFile())
	if removeData {
		os.RemoveAll(appDataDir())
	}
	dir := installDir()
	// this exe may be the one being removed, so a detached helper deletes the folder after it exits
	helper := exec.Command("/bin/sh", "-c", fmt.Sprintf("sleep 3; rm -rf %s", shellQuote(dir)))
	helper.Dir = os.TempDir()
	helper.Start()
	return 0
}

// waitForPID waits for the copy that started this one to exit.
func waitForPID(pid int) {
	for i := 0; i < 150; i++ {
		if syscall.Kill(pid, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func startUninstall() error {
	exe := installedExe()
	if !fileExists(exe) {
		return errors.New("SWIRL Card Manager is not installed on this PC")
	}
	return exec.Command(exe, "-uninstall").Start()
}

// ---------- updates ----------

const canSelfUpdate = true

// handOverToUpdate gunzips the downloaded binary and starts it; once this copy has quit it replaces
// the installed binary with itself and opens.
func handOverToUpdate(gzPath string) error {
	f, err := os.Open(gzPath)
	if err != nil {
		return fmt.Errorf("the download could not be opened (%v)", err)
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return fmt.Errorf("the download could not be opened (%v)", err)
	}
	dir := filepath.Join(updatesDir(), "new")
	os.MkdirAll(dir, 0o755)
	exe := filepath.Join(dir, "SWIRL-Card-Manager")
	out, err := os.Create(exe)
	if err != nil {
		zr.Close()
		f.Close()
		return err
	}
	if _, err := io.Copy(out, zr); err != nil {
		out.Close()
		zr.Close()
		f.Close()
		return err
	}
	out.Close()
	zr.Close()
	f.Close()
	os.Chmod(exe, 0o755)
	cmd := exec.Command(exe, "-apply-update", "-wait-pid", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("the new version could not be started (%v)", err)
	}
	go func() {
		time.Sleep(2500 * time.Millisecond) // lets the page show that the download finished
		quitNow()
	}()
	return nil
}

// applyUpdate runs in the downloaded binary after the old copy has quit: it installs itself over the
// installed copy and starts it.
func applyUpdate() int {
	if err := InstallApp(false); err != nil {
		fmt.Println("the update could not be installed:", err)
		return 1
	}
	cmd := exec.Command(installedExe())
	cmd.Dir = installDir()
	if err := cmd.Start(); err != nil {
		fmt.Println("Updated, but the app could not be started. Start SWIRL Card Manager from your applications menu.")
	}
	return 0
}
