//go:build windows

package main

import (
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
	"unsafe"
)

const appName = "SWIRL Card Manager"
const uninstallKey = `HKCU\Software\Microsoft\Windows\CurrentVersion\Uninstall\SWIRLCardManager`

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	messageBox       = user32.NewProc("MessageBoxW")
	systemParamsInfo = user32.NewProc("SystemParametersInfoW")
	getDpiForSystem  = user32.NewProc("GetDpiForSystem")
)

func installDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	return filepath.Join(base, "Programs", appName)
}

func installedExe() string { return filepath.Join(installDir(), appName+".exe") }

func selfExe() string {
	p, _ := os.Executable()
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
}

func runningInstalled() bool {
	return strings.EqualFold(filepath.Clean(selfExe()), filepath.Clean(installedExe()))
}

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
		Version: version, Platform: "windows", Installed: iv != "", InstalledVersion: iv,
		RunningInstalled: runningInstalled(), CanInstall: true, Window: windowKind, InstallDir: installDir(),
		DataBytes: dirBytes(appDataDir()),
	}
}

// ---------- the window ----------

var windowKind = "browser"

func findBrowser() (string, string) {
	var cands []string
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "LOCALAPPDATA"} {
		if b := os.Getenv(env); b != "" {
			cands = append(cands, filepath.Join(b, `Microsoft\Edge\Application\msedge.exe`))
		}
	}
	for _, c := range cands {
		if fileExists(c) {
			return c, "Edge"
		}
	}
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if b := os.Getenv(env); b != "" {
			if c := filepath.Join(b, `Google\Chrome\Application\chrome.exe`); fileExists(c) {
				return c, "Chrome"
			}
		}
	}
	return "", ""
}

// windowSize picks a comfortable size: 80 percent of the screen's work area, in the browser's units.
func windowSize() (x, y, w, h int) {
	type rect struct{ l, t, r, b int32 }
	var wa rect
	systemParamsInfo.Call(0x0030, 0, uintptr(unsafe.Pointer(&wa)), 0) // SPI_GETWORKAREA
	scale := 1.0
	if getDpiForSystem.Find() == nil {
		if d, _, _ := getDpiForSystem.Call(); d > 0 {
			scale = float64(d) / 96
		}
	}
	ww, wh := float64(wa.r-wa.l)/scale, float64(wa.b-wa.t)/scale
	if ww < 800 || wh < 600 {
		return 0, 0, 1280, 860
	}
	w, h = int(ww*0.8), int(wh*0.85)
	if w < 1100 {
		w = int(ww)
	}
	if h < 720 {
		h = int(wh)
	}
	return int(float64(wa.l)/scale + (ww-float64(w))/2), int(float64(wa.t)/scale + (wh-float64(h))/2), w, h
}

// openWindow shows the app in its own Edge (or Chrome) window with no address bar or tabs. It uses a
// profile of its own, so it never mixes with the person's browsing. Without either browser it opens a tab.
func openWindow(url string) {
	exe, _ := findBrowser()
	if exe == "" {
		windowKind = "browser"
		openBrowser(url)
		return
	}
	x, y, w, h := windowSize()
	prof := filepath.Join(appDataDir(), "window")
	args := []string{
		"--app=" + url,
		"--user-data-dir=" + prof,
		fmt.Sprintf("--window-size=%d,%d", w, h),
		fmt.Sprintf("--window-position=%d,%d", x, y),
		"--no-first-run", "--no-default-browser-check", "--disable-sync", "--disable-extensions",
		"--disable-features=msEdgeSidebarV2,msHubApps,msImplicitSignin,Translate,msUndersideButton",
		"--hide-crash-restore-bubble",
	}
	cmd := exec.Command(exe, args...)
	if err := cmd.Start(); err != nil {
		windowKind = "browser"
		openBrowser(url)
		return
	}
	windowKind = "app"
	go cmd.Wait() // the page itself says when it closes (see pageClosed)
}

// ---------- install, update, uninstall ----------

func reg(args ...string) error {
	cmd := exec.Command("reg", args...)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("reg %s: %v %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

func powershell(script string) error {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	hideWindow(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func shortcutPaths() []string {
	var out []string
	if a := os.Getenv("APPDATA"); a != "" {
		out = append(out, filepath.Join(a, `Microsoft\Windows\Start Menu\Programs`, appName+".lnk"))
	}
	if d := knownFolder(knownGUID{0xB4BFCC3A, 0xDB2C, 0x424C, [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41}}); d != "" {
		out = append(out, filepath.Join(d, appName+".lnk"))
	}
	return out
}

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
	// a running installed copy keeps its file locked; it can still be renamed out of the way
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
	os.Remove(old) // fails quietly while that copy is still running; cleaned up next time
	return nil
}

// InstallApp copies this exe into the user's Programs folder (no administrator rights needed), adds Start
// menu and desktop shortcuts, and registers it under Settings > Apps so it can be uninstalled.
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
	// Settings > Apps entry
	size := strconv.FormatInt(dirBytes(dir)/1024, 10)
	vals := [][]string{
		{"DisplayName", "REG_SZ", appName},
		{"DisplayVersion", "REG_SZ", version},
		{"Publisher", "REG_SZ", "Glen Huszar"},
		{"URLInfoAbout", "REG_SZ", "https://github.com/TheGlengineer"},
		{"DisplayIcon", "REG_SZ", exe + ",0"},
		{"InstallLocation", "REG_SZ", dir},
		{"UninstallString", "REG_SZ", `"` + exe + `" -uninstall`},
		{"QuietUninstallString", "REG_SZ", `"` + exe + `" -uninstall -quiet`},
		{"NoModify", "REG_DWORD", "1"},
		{"NoRepair", "REG_DWORD", "1"},
		{"EstimatedSize", "REG_DWORD", size},
		{"InstallDate", "REG_SZ", time.Now().Format("20060102")},
	}
	for _, v := range vals {
		if err := reg("add", uninstallKey, "/v", v[0], "/t", v[1], "/d", v[2], "/f"); err != nil {
			return err
		}
	}
	// shortcuts
	links := shortcutPaths()
	if !desktop && len(links) > 1 {
		os.Remove(links[1])
		links = links[:1]
	}
	var ps strings.Builder
	ps.WriteString("$w=New-Object -ComObject WScript.Shell;")
	for _, l := range links {
		fmt.Fprintf(&ps, "$s=$w.CreateShortcut(%s);$s.TargetPath=%s;$s.WorkingDirectory=%s;$s.IconLocation=%s;$s.Description=%s;$s.Save();",
			psQuote(l), psQuote(exe), psQuote(dir), psQuote(exe+",0"), psQuote("Set up GDEMU SD cards with the SWIRL menu"))
	}
	if err := powershell(ps.String()); err != nil {
		return fmt.Errorf("installed, but the Start menu and desktop shortcuts could not be made (%v); it can be started from %s", err, exe)
	}
	return nil
}

// SwitchToInstalled starts the installed copy once this one has quit, so the window reopens from there.
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

func msgBox(text string, flags uintptr) int {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(appName)
	r, _, _ := messageBox.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
	return int(r)
}

// Uninstall runs from Settings > Apps (or the app's own Uninstall button).
func Uninstall(quiet bool) int {
	const yesNo, yesNoCancel, question, info = 0x4, 0x3, 0x20, 0x40
	const idYes, idNo = 6, 7
	if !quiet && msgBox("Remove SWIRL Card Manager from this PC?\n\nYour SD cards and games are not touched.", yesNo|question) != idYes {
		return 1
	}
	removeData := quiet
	if !quiet {
		mb := float64(dirBytes(appDataDir())) / (1 << 20)
		switch msgBox(fmt.Sprintf("Also delete the art database, emulator files and settings it downloaded or unpacked (%.0f MB)?\n\nChoose No to keep them for a later install.", mb), yesNoCancel|question) {
		case idYes:
			removeData = true
		case idNo:
		default:
			return 1
		}
	}
	if ri := otherInstance(); ri != nil {
		askToQuit(ri)
	}
	for _, l := range shortcutPaths() {
		os.Remove(l)
	}
	reg("delete", uninstallKey, "/f")
	if removeData {
		os.RemoveAll(appDataDir())
	}
	dir := installDir()
	// this exe may be the one being removed, so a short-lived helper deletes the folder after it exits
	helper := exec.Command("cmd.exe")
	// cmd.exe needs its own quoting, so the command line is passed as is
	helper.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000 | 0x00000008, // CREATE_NO_WINDOW | DETACHED_PROCESS
		CmdLine:       `cmd.exe /c ping -n 3 127.0.0.1 >nul & rmdir /s /q "` + dir + `"`,
	}
	helper.Dir = os.TempDir()
	helper.Start()
	if !quiet {
		msgBox("SWIRL Card Manager was removed.", info)
	}
	return 0
}

// waitForPID waits for the copy that started this one to exit (used when switching to the installed copy).
func waitForPID(pid int) {
	const synchronize = 0x00100000
	h, err := syscall.OpenProcess(synchronize, false, uint32(pid))
	if err != nil {
		return
	}
	defer syscall.CloseHandle(h)
	syscall.WaitForSingleObject(h, 15000)
}

// startUninstall runs the uninstaller in its own process (it asks its questions in Windows dialogs and
// closes this copy of the app first).
func startUninstall() error {
	exe := installedExe()
	if !fileExists(exe) {
		return errors.New("SWIRL Card Manager is not installed on this PC")
	}
	return exec.Command(exe, "-uninstall").Start()
}

// ---------- updates ----------

const canSelfUpdate = true

// handOverToUpdate starts the downloaded exe, which installs itself once this copy has quit and then
// opens the installed copy.
func handOverToUpdate(exe string) error {
	cmd := exec.Command(exe, "-apply-update", "-wait-pid", strconv.Itoa(os.Getpid()))
	cmd.Dir = filepath.Dir(exe)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("the new version could not be started (%v)", err)
	}
	go func() {
		time.Sleep(2500 * time.Millisecond) // lets the page show that the download finished
		quitNow()
	}()
	return nil
}

// applyUpdate runs in the downloaded exe after the old copy has quit: it installs itself over the installed
// copy (keeping the desktop shortcut choice) and starts it.
func applyUpdate() int {
	desktop := true
	if installedVersion() != "" {
		links := shortcutPaths()
		desktop = len(links) > 1 && fileExists(links[1])
	}
	if err := InstallApp(desktop); err != nil {
		msgBox("The update could not be installed:\n\n"+err.Error(), 0x10)
		return 1
	}
	cmd := exec.Command(installedExe())
	cmd.Dir = installDir()
	if err := cmd.Start(); err != nil {
		msgBox("Updated, but the app could not be started. Start SWIRL Card Manager from the Start menu.", 0x40)
	}
	return 0
}
