package main

// The app around the web pages: one copy running at a time, its own window, closing when the window
// closes, and installing itself on the PC.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func appDataDir() string { return filepath.Dir(dbDir()) }

type runningInfo struct {
	Port    int    `json:"port"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
	Version string `json:"version"`
	Exe     string `json:"exe,omitempty"`  // the program file that is running
	Menu    string `json:"menu,omitempty"` // hash of the menu it carries
}

func thisExe() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	return p
}

// sameBuild reports whether the running copy is this very program: same version, same file, same menu inside.
// A different file with the same version number (a test build), or a copy too old to say what it is, is
// replaced, so the card gets the new menu.
func (ri *runningInfo) sameBuild() bool {
	return ri.Version == version && ri.Exe != "" && ri.Exe == thisExe() && ri.Menu == swirlHash(swirlBinary)
}

func runningPath() string { return filepath.Join(appDataDir(), "running.json") }

// otherInstance returns the copy of the app that is already running, if it answers.
func otherInstance() *runningInfo {
	b, err := os.ReadFile(runningPath())
	if err != nil {
		return nil
	}
	var ri runningInfo
	if json.Unmarshal(b, &ri) != nil || ri.Port == 0 || ri.PID == os.Getpid() {
		return nil
	}
	req, _ := http.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/ping", ri.Port), nil)
	req.Header.Set("X-Swirl-Token", ri.Token)
	c := &http.Client{Timeout: 2 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return nil
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	return &ri
}

func askToQuit(ri *runningInfo) bool {
	req, _ := http.NewRequest("POST", fmt.Sprintf("http://127.0.0.1:%d/api/quit", ri.Port), strings.NewReader("{}"))
	req.Header.Set("X-Swirl-Token", ri.Token)
	c := &http.Client{Timeout: 3 * time.Second}
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return false
	}
	for i := 0; i < 30; i++ {
		time.Sleep(200 * time.Millisecond)
		if otherInstance() == nil {
			return true
		}
	}
	return false
}

func writeRunning(port int) {
	os.MkdirAll(appDataDir(), 0o755)
	b, _ := json.Marshal(runningInfo{Port: port, Token: token, PID: os.Getpid(), Version: version, Exe: thisExe(), Menu: swirlHash(swirlBinary)})
	os.WriteFile(runningPath(), b, 0o600)
}

func clearRunning() {
	b, err := os.ReadFile(runningPath())
	if err != nil {
		return
	}
	var ri runningInfo
	if json.Unmarshal(b, &ri) == nil && ri.PID == os.Getpid() {
		os.Remove(runningPath())
	}
}

// busy is true while something is being written to a card; the app never quits then.
func busy() bool {
	jobMu.Lock()
	running := job.Running
	jobMu.Unlock()
	if running || anyCardBusy() {
		return true
	}
	if !mu.TryLock() {
		return true
	}
	mu.Unlock()
	return false
}

var (
	quitMu sync.Mutex
	byeAt  time.Time
)

func quitNow() {
	clearRunning()
	os.Exit(0)
}

// pageClosed is called when the page says goodbye (window or tab closed, or reloading). If no page comes
// back within a few seconds, the app quits, but never in the middle of writing to a card.
func pageClosed() {
	quitMu.Lock()
	byeAt = time.Now()
	quitMu.Unlock()
	go func() {
		time.Sleep(5 * time.Second)
		for {
			quitMu.Lock()
			came := lastPing.After(byeAt)
			quitMu.Unlock()
			if came {
				return
			}
			if !busy() {
				quitNow()
			}
			time.Sleep(2 * time.Second)
		}
	}()
}

// idleWatch quits a while after the last page stopped talking to the app (a crashed browser, say).
func idleWatch() {
	for range time.Tick(10 * time.Second) {
		if time.Since(lastPing) > 3*time.Minute && !busy() {
			quitNow()
		}
	}
}

type AppInfo struct {
	Version          string `json:"version"`
	Platform         string `json:"platform"`
	Installed        bool   `json:"installed"`        // a copy is installed on this PC
	InstalledVersion string `json:"installedVersion"` // its version
	RunningInstalled bool   `json:"runningInstalled"` // this is the installed copy
	CanInstall       bool   `json:"canInstall"`
	Window           string `json:"window"` // app, browser
	InstallDir       string `json:"installDir,omitempty"`
	DataBytes        int64  `json:"dataBytes"`
	DataDir          string `json:"dataDir,omitempty"`
}

func dirBytes(p string) int64 {
	var n int64
	filepath.Walk(p, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n += info.Size()
		}
		return nil
	})
	return n
}

// versionNewer reports whether a is a later version than b (2.10 > 2.9). A preview such as 2.14.0-preview.1
// comes before its release 2.14.0 and after 2.13.x; previews of one version are ordered by their number.
func versionNewer(a, b string) bool {
	ca, pa := splitVersion(a)
	cb, pb := splitVersion(b)
	na, nb := strings.Split(ca, "."), strings.Split(cb, ".")
	for i := 0; i < len(na) || i < len(nb); i++ {
		var x, y int
		if i < len(na) {
			fmt.Sscan(na[i], &x)
		}
		if i < len(nb) {
			fmt.Sscan(nb[i], &y)
		}
		if x != y {
			return x > y
		}
	}
	switch {
	case pa == pb:
		return false
	case pa == "": // the release itself is newer than any of its previews
		return true
	case pb == "":
		return false
	}
	return previewNumber(pa) > previewNumber(pb)
}

// splitVersion splits "2.14.0-preview.1" into "2.14.0" and "preview.1".
func splitVersion(v string) (core, pre string) {
	v = strings.TrimSpace(v)
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}
	return v, ""
}

func previewNumber(pre string) int {
	n := 0
	if i := strings.LastIndexByte(pre, '.'); i >= 0 {
		fmt.Sscan(pre[i+1:], &n)
	}
	return n
}

// isPreview reports whether v is a preview version (2.14.0-preview.1).
func isPreview(v string) bool {
	_, pre := splitVersion(v)
	return pre != ""
}

// versionLabel is how a version reads to people: "2.14.0 preview 1".
func versionLabel(v string) string {
	core, pre := splitVersion(v)
	if pre == "" {
		return core
	}
	return core + " preview " + fmt.Sprint(previewNumber(pre))
}
