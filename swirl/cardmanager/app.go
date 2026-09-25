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
	b, _ := json.Marshal(runningInfo{Port: port, Token: token, PID: os.Getpid(), Version: version})
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
	if running {
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

// versionNewer reports whether a is a later version than b (2.10 > 2.9).
func versionNewer(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			fmt.Sscan(pa[i], &x)
		}
		if i < len(pb) {
			fmt.Sscan(pb[i], &y)
		}
		if x != y {
			return x > y
		}
	}
	return false
}
