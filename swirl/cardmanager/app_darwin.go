//go:build darwin

package main

// SWIRL Card Manager on macOS: a normal .app bundle. It has no Dock icon of its own (LSUIElement); its
// window is Chrome, Edge or Brave in app mode (or a Safari tab), exactly as Edge is used on Windows.
// Installing copies the bundle into Applications; updating swaps the bundle for the downloaded one.

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
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

// bundleRoot is the .app folder this program runs from, or "" when it runs as a bare binary.
func bundleRoot() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	i := strings.Index(exe, ".app/Contents/MacOS/")
	if i < 0 {
		return ""
	}
	return exe[:i+4]
}

// translocated reports whether macOS runs the app from a temporary read-only copy (a freshly downloaded
// app that was not moved into Applications).
func translocated(p string) bool { return strings.Contains(p, "/AppTranslocation/") }

func installDir() string {
	if st, err := os.Stat("/Applications"); err == nil && st.IsDir() && writable("/Applications") {
		return "/Applications"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Applications")
}

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

func installedApp() string {
	for _, d := range []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")} {
		p := filepath.Join(d, appName+".app")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
	}
	return filepath.Join(installDir(), appName+".app")
}

func bundleVersion(app string) string {
	b, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		return ""
	}
	v, err := parsePlist(bytes.NewReader(b))
	if err != nil {
		return ""
	}
	m, _ := v.(map[string]any)
	return plistDict(m).str("CFBundleShortVersionString")
}

func installedVersion() string { return bundleVersion(installedApp()) }

func samePath(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

func runningInstalled() bool {
	b := bundleRoot()
	return b != "" && samePath(b, installedApp())
}

func getAppInfo() AppInfo {
	iv := installedVersion()
	return AppInfo{
		Version: version, Platform: "darwin", Installed: iv != "", InstalledVersion: iv,
		RunningInstalled: runningInstalled(), CanInstall: bundleRoot() != "", Window: windowKind,
		InstallDir: installedApp(), DataBytes: dirBytes(appDataDir()),
	}
}

// ---------- the window ----------

func findBrowser() string {
	home, _ := os.UserHomeDir()
	for _, name := range []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Chromium", "Vivaldi"} {
		for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
			p := filepath.Join(dir, name+".app", "Contents", "MacOS", name)
			if fileExists(p) {
				return p
			}
		}
	}
	return ""
}

// openWindow shows the app in its own Chrome, Edge or Brave window with no address bar, using a
// profile of its own. Without any of them it opens a tab in the default browser.
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

// replaceBundle puts a copy of src at dst, swapping it in only once the copy is complete.
func replaceBundle(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".new"
	os.RemoveAll(tmp)
	if err := copyTreeKeep(src, tmp); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	old := dst + ".old"
	os.RemoveAll(old)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, old); err != nil {
			os.RemoveAll(tmp)
			return fmt.Errorf("the installed copy could not be replaced (%v)", err)
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		os.Rename(old, dst)
		return err
	}
	os.RemoveAll(old)
	// a copy made by this app carries no download quarantine, but clear it anyway
	exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", dst).Run()
	return nil
}

// InstallApp copies this app into Applications (or ~/Applications when Applications is not writable).
func InstallApp(desktop bool) error {
	src := bundleRoot()
	if src == "" {
		return errors.New("start SWIRL Card Manager from its .app to install it")
	}
	if runningInstalled() {
		return errors.New("this is already the installed copy")
	}
	dst := filepath.Join(installDir(), appName+".app")
	if err := replaceBundle(src, dst); err != nil {
		return err
	}
	return nil
}

// SwitchToInstalled opens the installed copy once this one has quit.
func SwitchToInstalled() error {
	app := installedApp()
	if !fileExists(filepath.Join(app, "Contents", "Info.plist")) {
		return errors.New("SWIRL Card Manager is not installed")
	}
	exe := filepath.Join(app, "Contents", "MacOS", appName)
	cmd := exec.Command(exe, "-wait-pid", strconv.Itoa(os.Getpid()))
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() {
		time.Sleep(400 * time.Millisecond)
		quitNow()
	}()
	return nil
}

// dialog shows a macOS dialog and returns the button pressed ("" when cancelled).
func dialog(text string, buttons ...string) string {
	q := make([]string, len(buttons))
	for i, b := range buttons {
		q[i] = appleScriptQuote(b)
	}
	script := fmt.Sprintf("display dialog %s buttons {%s} default button %s with title %s",
		appleScriptQuote(text), strings.Join(q, ", "), q[len(q)-1], appleScriptQuote(appName))
	out, err := exec.Command("/usr/bin/osascript", "-e", script).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(out)), "button returned:"))
}

func appleScriptQuote(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// moveToTrash puts a file or folder in the Trash.
func moveToTrash(p string) error {
	home, _ := os.UserHomeDir()
	trash := filepath.Join(home, ".Trash")
	dst := filepath.Join(trash, filepath.Base(p))
	for i := 2; fileExists(dst) || isDir(dst); i++ {
		dst = filepath.Join(trash, fmt.Sprintf("%s %d%s", strings.TrimSuffix(filepath.Base(p), filepath.Ext(p)), i, filepath.Ext(p)))
	}
	if err := os.Rename(p, dst); err != nil {
		return os.RemoveAll(p)
	}
	return nil
}

// Uninstall removes the installed app (to the Trash) and, if asked, the data it downloaded.
func Uninstall(quiet bool) int {
	if !quiet && dialog("Remove SWIRL Card Manager from this Mac?\n\nYour SD cards and games are not touched.", "Cancel", "Remove") != "Remove" {
		return 1
	}
	removeData := quiet
	if !quiet {
		mb := float64(dirBytes(appDataDir())) / (1 << 20)
		switch dialog(fmt.Sprintf("Also delete the art database, emulator files and settings it downloaded or unpacked (%.0f MB)?", mb), "Cancel", "Keep them", "Delete them") {
		case "Delete them":
			removeData = true
		case "Keep them":
		default:
			return 1
		}
	}
	if ri := otherInstance(); ri != nil {
		askToQuit(ri)
	}
	if app := installedApp(); isDir(app) {
		moveToTrash(app)
	}
	if removeData {
		os.RemoveAll(appDataDir())
	}
	if !quiet {
		dialog("SWIRL Card Manager was moved to the Trash.", "OK")
	}
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
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return exec.Command(exe, "-uninstall").Start()
}

// ---------- updates ----------

const canSelfUpdate = true

// handOverToUpdate unpacks the downloaded app and starts it; once this copy has quit it replaces this app
// (or the installed one) with itself and opens.
func handOverToUpdate(zipPath string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("the download could not be opened (%v)", err)
	}
	dir := filepath.Join(updatesDir(), "new")
	os.RemoveAll(dir)
	err = extractZipTree(&zr.Reader, dir)
	zr.Close()
	if err != nil {
		return err
	}
	app := findApp(dir)
	if app == "" {
		return errors.New("the download does not contain the app")
	}
	target := bundleRoot()
	if target == "" || translocated(target) || !writable(filepath.Dir(target)) {
		target = filepath.Join(installDir(), appName+".app")
	}
	exe := filepath.Join(app, "Contents", "MacOS", appName)
	cmd := exec.Command(exe, "-apply-update", "-wait-pid", strconv.Itoa(os.Getpid()))
	cmd.Env = append(os.Environ(), "SWIRL_UPDATE_TARGET="+target)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("the new version could not be started (%v)", err)
	}
	go func() {
		time.Sleep(2500 * time.Millisecond)
		quitNow()
	}()
	return nil
}

// applyUpdate runs in the unpacked new version: it copies itself over the old app and opens it.
func applyUpdate() int {
	target := os.Getenv("SWIRL_UPDATE_TARGET")
	if target == "" {
		target = filepath.Join(installDir(), appName+".app")
	}
	src := bundleRoot()
	if src == "" {
		return 1
	}
	if err := replaceBundle(src, target); err != nil {
		dialog("The update could not be installed:\n\n"+err.Error(), "OK")
		return 1
	}
	if err := exec.Command("/usr/bin/open", "-n", target).Start(); err != nil {
		dialog("Updated, but the app could not be opened. Open SWIRL Card Manager from Applications.", "OK")
	}
	return 0
}
