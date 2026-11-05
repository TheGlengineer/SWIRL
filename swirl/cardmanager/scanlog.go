package main

// What a scan is doing, as it does it (2.17.1). Every ScanCard writes a line per step to the scan log:
// in memory for the window's progress panel and /api/scan/status, and to scan.log in the app's data
// folder so a scan that never finished still leaves a trace of the folder it stopped on. One disc that
// cannot be read in time is skipped with a warning naming it, so the rest of the card still opens; a
// panic inside the scan becomes an error with the log attached instead of a window that sits on
// "Reading". /api/scan/report puts it all in one text the owner can paste into a GitHub issue.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// scanFolderLimit is how long one game folder may take before it is skipped. A CDI whose header sits at
// the end is read in 1 MB steps up to 128 MB, which a USB 2 reader does in well under this.
var scanFolderLimit = 30 * time.Second

// scanSlow marks a folder as slow in the log and the summary.
const scanSlow = 2 * time.Second

type scanFolderTime struct {
	Folder string `json:"folder"`
	Name   string `json:"name"`
	Ms     int64  `json:"ms"`
}

// scanStatus is what /api/scan/status reports: the scan that is running, or the last one.
type scanStatus struct {
	Running  bool             `json:"running"`
	Root     string           `json:"root"`
	Stage    string           `json:"stage"`
	Folder   string           `json:"folder,omitempty"` // the game folder being read
	Done     int              `json:"done"`
	Total    int              `json:"total"`
	Pct      float64          `json:"pct"`
	Ms       int64            `json:"ms"` // since the scan started
	Log      []string         `json:"log"`
	Error    string           `json:"error,omitempty"`
	Skipped  []string         `json:"skipped,omitempty"` // folders that went past scanFolderLimit
	Slowest  []scanFolderTime `json:"slowest,omitempty"`
	Started  string           `json:"started"`
	Finished bool             `json:"finished"`
}

type scanLog struct {
	mu      sync.Mutex
	st      scanStatus
	start   time.Time
	times   []scanFolderTime
	file    *os.File
	stageAt time.Time
}

var (
	scanMu   sync.Mutex
	scanLast *scanLog // the scan running, or the last one
)

func scanLogPath() string { return filepath.Join(appDataDir(), "scan.log") }

// newScanLog starts a log for a scan of root. The file is written as the scan goes, a line at a time.
func newScanLog(root string) *scanLog {
	now := time.Now()
	l := &scanLog{start: now, stageAt: now}
	l.st = scanStatus{Running: true, Root: root, Stage: "Starting", Started: now.Format(time.RFC3339), Log: []string{}}
	os.MkdirAll(appDataDir(), 0o755)
	if f, err := os.Create(scanLogPath()); err == nil {
		l.file = f
		fmt.Fprintf(f, "SWIRL Card Manager %s scan log, %s/%s, started %s\ncard: %s\n", version, runtime.GOOS, runtime.GOARCH, now.Format(time.RFC3339), root)
	}
	scanMu.Lock()
	scanLast = l
	scanMu.Unlock()
	return l
}

func (l *scanLog) line(format string, args ...any) {
	if l == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	ms := time.Since(l.start).Milliseconds()
	l.mu.Lock()
	l.st.Log = append(l.st.Log, msg)
	l.st.Ms = ms
	if l.file != nil {
		fmt.Fprintf(l.file, "%7d ms  %s\n", ms, msg)
	}
	l.mu.Unlock()
}

// stage names the step the scan is on; the time the previous step took goes to the log.
func (l *scanLog) stage(name string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	prev, took := l.st.Stage, time.Since(l.stageAt)
	l.st.Stage, l.st.Folder = name, ""
	l.stageAt = time.Now()
	l.mu.Unlock()
	if prev != "" && prev != "Starting" && took > scanSlow {
		l.line("%s took %.1f s", prev, took.Seconds())
	}
	l.line("%s", name)
}

func (l *scanLog) total(n int) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.st.Total = n
	l.mu.Unlock()
}

// folder notes the game folder being read now.
func (l *scanLog) folder(name string, idx int) {
	if l == nil {
		return
	}
	l.mu.Lock()
	l.st.Folder, l.st.Done = name, idx
	if l.st.Total > 0 {
		l.st.Pct = float64(idx) / float64(l.st.Total)
	}
	l.st.Ms = time.Since(l.start).Milliseconds()
	l.mu.Unlock()
}

// folderDone records what reading one folder gave and how long it took.
func (l *scanLog) folderDone(folder string, g *Game, took time.Duration, skipped bool) {
	if l == nil {
		return
	}
	ms := took.Milliseconds()
	what := g.Format
	if g.Type == "psx" {
		what = "PlayStation"
	}
	if what == "" {
		what = "no disc image"
	}
	note := ""
	switch {
	case skipped:
		note = "  SKIPPED: " + g.Error
	case g.Error != "":
		note = "  cannot read: " + g.Error
	case took > scanSlow:
		note = "  slow"
	}
	l.line("%s  %s  %s  %d ms%s", folder, g.Name, what, ms, note)
	l.mu.Lock()
	l.times = append(l.times, scanFolderTime{Folder: folder, Name: g.Name, Ms: ms})
	l.st.Done++
	if l.st.Total > 0 {
		l.st.Pct = float64(l.st.Done) / float64(l.st.Total)
	}
	if skipped {
		l.st.Skipped = append(l.st.Skipped, folder)
	}
	l.mu.Unlock()
}

// finish closes the log with the result.
func (l *scanLog) finish(c *Card, err error) {
	if l == nil {
		return
	}
	took := time.Since(l.start)
	l.mu.Lock()
	sort.Slice(l.times, func(i, j int) bool { return l.times[i].Ms > l.times[j].Ms })
	if len(l.times) > 5 {
		l.st.Slowest = append([]scanFolderTime(nil), l.times[:5]...)
	} else {
		l.st.Slowest = append([]scanFolderTime(nil), l.times...)
	}
	l.mu.Unlock()
	if err != nil {
		l.line("Stopped after %.1f s: %s", took.Seconds(), err.Error())
	} else {
		l.line("Done in %.1f s: %d game folders, %d warnings", took.Seconds(), len(c.Games), len(c.Warnings))
		for _, w := range c.Warnings {
			l.line("warning: %s", w)
		}
	}
	l.mu.Lock()
	l.st.Running, l.st.Finished, l.st.Folder, l.st.Stage = false, true, "", "Done"
	if err != nil {
		l.st.Error = err.Error()
		l.st.Stage = "Stopped"
	} else {
		l.st.Pct = 1
	}
	l.st.Ms = took.Milliseconds()
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
	l.mu.Unlock()
}

func (l *scanLog) snapshot() scanStatus {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.st
	s.Log = append([]string(nil), l.st.Log...)
	s.Skipped = append([]string(nil), l.st.Skipped...)
	s.Slowest = append([]scanFolderTime(nil), l.st.Slowest...)
	if s.Running {
		s.Ms = time.Since(l.start).Milliseconds()
	}
	return s
}

// ScanStatus is the running scan, or the last one; an empty status before any.
func ScanStatus() scanStatus {
	scanMu.Lock()
	l := scanLast
	scanMu.Unlock()
	if l == nil {
		return scanStatus{Log: []string{}}
	}
	return l.snapshot()
}

// readFolderInTime runs fn, and gives up after scanFolderLimit. The read keeps going in the background
// until the operating system answers it (a read cannot be interrupted); its result is dropped.
func readFolderInTime(fn func()) (timedOut bool) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
		return false
	case <-time.After(scanFolderLimit):
		return true
	}
}

// scanRecover turns a panic inside a scan into an error that names where it happened.
func scanRecover(l *scanLog, errp *error) {
	if r := recover(); r != nil {
		where := ""
		if l != nil {
			l.mu.Lock()
			where = l.st.Stage
			if l.st.Folder != "" {
				where += ", folder " + l.st.Folder
			}
			l.mu.Unlock()
		}
		msg := fmt.Sprintf("Card Manager hit an unexpected problem while reading the card (%s): %v", where, r)
		l.line("%s", msg)
		for _, ln := range strings.Split(string(debug.Stack()), "\n") {
			if ln = strings.TrimSpace(ln); ln != "" {
				l.line("  %s", ln)
			}
		}
		*errp = errors.New(msg + ". The scan log in Report a problem says more.")
	}
}

// ScanReport is the text for a GitHub issue: the program, the computer, the card's drive and the scan log.
func ScanReport(root string) string {
	s := ScanStatus()
	var b strings.Builder
	fmt.Fprintf(&b, "SWIRL Card Manager %s on %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "Card: %s\n", root)
	for _, d := range listDrives() {
		if strings.EqualFold(strings.TrimRight(d.Path, `\/`), strings.TrimRight(root, `\/`)) {
			fmt.Fprintf(&b, "Drive: %s, %s, %s", d.Path, d.Type, d.FS)
			if d.Removable {
				b.WriteString(", removable")
			}
			if d.Total > 0 {
				fmt.Fprintf(&b, ", %.1f GB with %.1f GB free", float64(d.Total)/1e9, float64(d.Free)/1e9)
			}
			if d.Label != "" {
				fmt.Fprintf(&b, ", label %q", d.Label)
			}
			b.WriteString("\n")
		}
	}
	if s.Started == "" {
		b.WriteString("No scan has run yet.\n")
		return b.String()
	}
	switch {
	case s.Running:
		fmt.Fprintf(&b, "Scan still running after %.1f s: %s", float64(s.Ms)/1000, s.Stage)
		if s.Folder != "" {
			fmt.Fprintf(&b, ", folder %s", s.Folder)
		}
		fmt.Fprintf(&b, " (%d of %d folders read)\n", s.Done, s.Total)
	case s.Error != "":
		fmt.Fprintf(&b, "Scan stopped after %.1f s: %s\n", float64(s.Ms)/1000, s.Error)
	default:
		fmt.Fprintf(&b, "Scan finished in %.1f s, %d folders\n", float64(s.Ms)/1000, s.Done)
	}
	if len(s.Skipped) > 0 {
		fmt.Fprintf(&b, "Folders skipped for taking more than %.0f s: %s\n", scanFolderLimit.Seconds(), strings.Join(s.Skipped, ", "))
	}
	if len(s.Slowest) > 0 {
		b.WriteString("Slowest folders:")
		for _, t := range s.Slowest {
			fmt.Fprintf(&b, " %s (%s) %d ms;", t.Folder, t.Name, t.Ms)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Scan log (%s):\n", scanLogPath())
	for _, ln := range s.Log {
		b.WriteString("  " + ln + "\n")
	}
	return b.String()
}

// SaveScanReport writes the report as a text file on the Desktop (the data folder when there is no
// Desktop) and returns its path.
func SaveScanReport(root string) (string, error) {
	dir := appDataDir()
	for _, pl := range places() {
		if pl.Name == "Desktop" && pl.Path != "" {
			if st, err := os.Stat(pl.Path); err == nil && st.IsDir() {
				dir = pl.Path
				break
			}
		}
	}
	path := filepath.Join(dir, "SWIRL-scan-report-"+time.Now().Format("2006-01-02-150405")+".txt")
	if err := os.WriteFile(path, []byte(ScanReport(root)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
