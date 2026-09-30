package main

// The menu's QR report, decoded (docs/DIAGNOSTICS.md). The console shows the report as one or more QR codes,
// each headed "SWIRL <version> <part>/<parts>"; the user scans them with a phone or photographs the screen.
// ParseReport takes the scanned text, in any order, and turns it into a Report the page can show in plain
// words. Warning codes come from assets/codes.tsv, a copy of swirl/diag/codes.tsv (a test keeps them equal).
// Crash addresses are symbolicated from the menu's ELF file when one for that build can be found locally.

import (
	"bufio"
	"bytes"
	"debug/elf"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

//go:embed assets/codes.tsv
var codesTSV string

// WarnCode is one row of codes.tsv.
type WarnCode struct {
	Code string `json:"code"` // W04
	Key  string `json:"key"`  // ini-line
	Text string `json:"text"` // OPENMENU.INI has lines SWIRL could not use
}

var (
	warnCodesOnce sync.Once
	warnCodeTable map[string]WarnCode
)

func warnCodes() map[string]WarnCode {
	warnCodesOnce.Do(func() {
		warnCodeTable = map[string]WarnCode{}
		sc := bufio.NewScanner(strings.NewReader(codesTSV))
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
				continue
			}
			f := strings.Split(line, "\t")
			if len(f) < 3 {
				continue
			}
			warnCodeTable[f[0]] = WarnCode{Code: f[0], Key: f[1], Text: f[2]}
		}
	})
	return warnCodeTable
}

// ReportDevice is one maple device from the d= line.
type ReportDevice struct {
	Port string `json:"port"` // A1
	Name string `json:"name"` // Visual Memory
	Kind string `json:"kind"` // controller, memory card, or the name when unknown
}

// ReportWarning is one counter from the w= line, decoded.
type ReportWarning struct {
	Code   string `json:"code"`
	Key    string `json:"key"`
	Text   string `json:"text"`   // plain words; "Unknown warning" when the code is not in the table
	Count  int    `json:"count"`  // how many times it happened
	Detail string `json:"detail"` // the first detail seen in the trace, if the trace still holds it
}

// ReportAddr is one code address from the x= line, with its function when symbols were found.
type ReportAddr struct {
	Addr   string `json:"addr"`             // 8c03271c
	Func   string `json:"func,omitempty"`   // main
	Offset int    `json:"offset,omitempty"` // bytes into the function
}

// ReportCrash is the x= line: only present for a crash or a hang.
type ReportCrash struct {
	PC    ReportAddr   `json:"pc"`
	PR    ReportAddr   `json:"pr"`
	Stack []ReportAddr `json:"stack"`
}

// TraceLine is one line of the t= section.
type TraceLine struct {
	MS   int    `json:"ms"`             // milliseconds since power on; -1 for a note inserted by the decoder
	Text string `json:"text"`           // the line, without the timestamp
	Code string `json:"code,omitempty"` // W04 when the line is a warning
}

// Report is the decoded report.
type Report struct {
	Version    string `json:"version"`    // 2.14.0-preview.3
	Build      string `json:"build"`      // a747bc2 (without -dirty)
	Dirty      bool   `json:"dirty"`      // built with uncommitted changes
	Reason     int    `json:"reason"`     // R1 to R7
	ReasonName string `json:"reasonName"` // crash, hang, diagnostics, boot log, assert, abort, launch
	ReasonText string `json:"reasonText"` // one sentence in plain words
	Summary    string `json:"summary"`    // a short paragraph in plain words

	BIOS       string `json:"bios"`
	Region     int    `json:"region"`
	RegionName string `json:"regionName"`
	Cable      int    `json:"cable"`
	CableName  string `json:"cableName"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	PixelMode  int    `json:"pixelMode"`
	VideoName  string `json:"videoName"`
	FreeKB     int    `json:"freeKB"`
	ArenaKB    int    `json:"arenaKB"`
	MemoryText string `json:"memoryText"`
	Uptime     int    `json:"uptime"` // seconds
	Games      int    `json:"games"`

	Devices     []ReportDevice  `json:"devices"`
	Warnings    []ReportWarning `json:"warnings"`
	Crash       *ReportCrash    `json:"crash,omitempty"`
	Symbols     string          `json:"symbols,omitempty"` // the symbols file used, or empty
	SymbolsNote string          `json:"symbolsNote,omitempty"`
	Trace       []TraceLine     `json:"trace"`
	LastStep    string          `json:"lastStep"` // the last trace line before the report was shown

	Parts   int      `json:"parts"`   // parts the console said there were (0 when no part headers were seen)
	Found   int      `json:"found"`   // parts seen
	Missing []int    `json:"missing"` // parts not seen
	Notes   []string `json:"notes"`   // what the decoder had to guess or skip
	Raw     string   `json:"raw"`     // the joined report text, part headers removed
}

var partHeader = regexp.MustCompile(`^SWIRL (\S+) (\d+)/(\d+)\s*$`)
var reportHeader = regexp.MustCompile(`^SWIRL (\S+) (\S+) R(\d+)\s*$`)
var traceLine = regexp.MustCompile(`^(\d+) (.*)$`)
var warnPrefix = regexp.MustCompile(`^(W\d\d)(?: (.*))?$`)
var devEntry = regexp.MustCompile(`^([A-D][0-5]):(.*)$`)
var hexAddr = regexp.MustCompile(`^[0-9a-fA-F]{6,8}$`)

var reasonNames = map[int][2]string{
	1: {"crash", "SWIRL crashed (a CPU exception). The screen stays until power off."},
	2: {"hang", "SWIRL made no progress for 12 seconds (no frame drawn, no trace line) and its watchdog stopped it."},
	3: {"diagnostics", "You opened System > Diagnostics and pressed A. Nothing went wrong by itself."},
	4: {"boot log", "X was held at power on, so SWIRL showed its start up log. Nothing went wrong by itself."},
	5: {"assert", "A check inside SWIRL failed (an assert). The screen stays until power off."},
	6: {"abort", "SWIRL stopped itself (abort). The screen stays until power off."},
	7: {"launch", "A game launch failed: the GDEMU did not answer and the menu disc did not come back."},
}

// joinParts sorts the scanned parts, drops duplicates, joins them and says what is missing.
func joinParts(text string) (body string, parts, found int, missing []int, notes []string) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	type part struct {
		n     int
		lines []string
	}
	var (
		list  []*part
		cur   *part
		head  []string
		total int
	)
	for _, line := range strings.Split(text, "\n") {
		if m := partHeader.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			n, _ := strconv.Atoi(m[2])
			t, _ := strconv.Atoi(m[3])
			if t > total {
				total = t
			}
			cur = &part{n: n}
			list = append(list, cur)
			continue
		}
		if cur == nil {
			head = append(head, line)
		} else {
			cur.lines = append(cur.lines, line)
		}
	}
	if len(list) == 0 {
		// no part headers: the whole text is the report (a serial capture, or one part with its header lost)
		return strings.Join(head, "\n"), 0, 0, nil, nil
	}
	if strings.TrimSpace(strings.Join(head, "\n")) != "" {
		notes = append(notes, "Text before the first part header was ignored.")
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].n < list[j].n })
	seen := map[int]bool{}
	var out []string
	last := 0
	for _, p := range list {
		if seen[p.n] {
			notes = append(notes, fmt.Sprintf("Part %d was scanned more than once; the first copy was used.", p.n))
			continue
		}
		seen[p.n] = true
		for n := last + 1; n < p.n; n++ {
			missing = append(missing, n)
			out = append(out, fmt.Sprintf("-1 (part %d of %d was not scanned)", n, total))
		}
		last = p.n
		out = append(out, strings.TrimRight(strings.Join(p.lines, "\n"), "\n"))
	}
	for n := last + 1; n <= total; n++ {
		missing = append(missing, n)
		out = append(out, fmt.Sprintf("-1 (part %d of %d was not scanned)", n, total))
	}
	for _, n := range missing {
		notes = append(notes, fmt.Sprintf("Part %d of %d is missing; the report has a gap there.", n, total))
	}
	return strings.Join(out, "\n"), total, len(seen), missing, notes
}

// ParseReport decodes the scanned text. It never fails on a partial report: what could not be read becomes
// a note. An error means the text does not look like a SWIRL report at all.
func ParseReport(text string) (*Report, error) {
	body, parts, found, missing, notes := joinParts(text)
	r := &Report{Parts: parts, Found: found, Missing: missing, Notes: notes, Raw: body, Devices: []ReportDevice{}, Warnings: []ReportWarning{}, Trace: []TraceLine{}}
	if r.Missing == nil {
		r.Missing = []int{}
	}
	if r.Notes == nil {
		r.Notes = []string{}
	}
	lines := strings.Split(body, "\n")
	inTrace := false
	sawHeader := false
	firstDetail := map[string]string{}
	addTrace := func(line string) {
		if strings.HasPrefix(line, "-1 (part ") {
			r.Trace = append(r.Trace, TraceLine{MS: -1, Text: strings.TrimPrefix(line, "-1 ")})
			return
		}
		m := traceLine.FindStringSubmatch(line)
		if m == nil {
			// a line without a timestamp: a wrapped line or scanner noise; keep it so nothing is lost
			r.Trace = append(r.Trace, TraceLine{MS: -1, Text: line})
			return
		}
		ms, _ := strconv.Atoi(m[1])
		t := TraceLine{MS: ms, Text: m[2]}
		if w := warnPrefix.FindStringSubmatch(m[2]); w != nil {
			t.Code = w[1]
			if _, ok := firstDetail[w[1]]; !ok {
				firstDetail[w[1]] = w[2]
			}
		}
		r.Trace = append(r.Trace, t)
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if inTrace {
			addTrace(line)
			continue
		}
		if m := reportHeader.FindStringSubmatch(line); m != nil && !sawHeader {
			sawHeader = true
			r.Version = m[1]
			r.Build = strings.TrimSuffix(m[2], "-dirty")
			r.Dirty = strings.HasSuffix(m[2], "-dirty")
			r.Reason, _ = strconv.Atoi(m[3])
			continue
		}
		if strings.HasPrefix(line, "-1 (part ") {
			// a missing part before the trace: the header fields it held are gone
			addTrace(line)
			continue
		}
		if !sawHeader && traceLine.MatchString(line) {
			// the first part is missing, so the "t=" line is too: this is where the trace begins
			inTrace = true
			addTrace(line)
			continue
		}
		switch {
		case strings.HasPrefix(line, "b="):
			r.parseFields(line)
		case strings.HasPrefix(line, "d="):
			r.parseDevices(line[2:])
		case strings.HasPrefix(line, "w="):
			r.parseWarnings(line[2:])
		case strings.HasPrefix(line, "x="):
			r.parseCrash(line[2:])
		case line == "t=":
			inTrace = true
		default:
			if !sawHeader {
				continue // text before the report, a Reddit comment for example
			}
			r.Notes = append(r.Notes, "A line the decoder did not expect was skipped: "+line)
		}
	}
	if !sawHeader && len(r.Trace) == 0 && len(r.Warnings) == 0 && len(r.Devices) == 0 {
		return nil, fmt.Errorf("this does not look like a SWIRL report: no \"SWIRL <version> <build> R<n>\" line and no trace")
	}
	if !sawHeader {
		r.Notes = append(r.Notes, "The first line (version, build and reason) was not scanned.")
	}
	for i := range r.Warnings {
		r.Warnings[i].Detail = firstDetail[r.Warnings[i].Code]
	}
	if r.Version == "" {
		// the second trace line is "SWIRL <version> starting"
		for _, t := range r.Trace {
			if strings.HasPrefix(t.Text, "SWIRL ") && strings.HasSuffix(t.Text, " starting") {
				r.Version = strings.TrimSuffix(strings.TrimPrefix(t.Text, "SWIRL "), " starting")
				break
			}
		}
	}
	if rn, ok := reasonNames[r.Reason]; ok {
		r.ReasonName, r.ReasonText = rn[0], rn[1]
	} else if sawHeader {
		r.ReasonName, r.ReasonText = "?", fmt.Sprintf("Reason R%d is not one this Card Manager knows; a newer SWIRL may have added it.", r.Reason)
	}
	for i := len(r.Trace) - 1; i >= 0; i-- {
		t := r.Trace[i]
		if t.MS < 0 || strings.HasPrefix(t.Text, "report shown") || strings.HasPrefix(t.Text, "STOPPED") {
			continue
		}
		r.LastStep = t.Text
		break
	}
	r.symbolicate()
	r.Summary = r.summary()
	return r, nil
}

func (r *Report) parseFields(line string) {
	for _, f := range strings.Fields(line) {
		k, v, ok := strings.Cut(f, "=")
		if !ok {
			continue
		}
		n, _ := strconv.Atoi(v)
		switch k {
		case "b":
			r.BIOS = v
		case "r":
			r.Region = n
			r.RegionName = map[int]string{0: "Japan", 1: "USA", 2: "Europe"}[n]
			if r.RegionName == "" {
				r.RegionName = "region " + v
			}
		case "c":
			r.Cable = n
			r.CableName = map[int]string{0: "VGA", 2: "RGB", 3: "composite"}[n]
			if r.CableName == "" {
				r.CableName = "cable " + v
			}
		case "v":
			wh, pm, _ := strings.Cut(v, "/")
			w, h, _ := strings.Cut(wh, "x")
			r.Width, _ = strconv.Atoi(w)
			r.Height, _ = strconv.Atoi(h)
			r.PixelMode, _ = strconv.Atoi(pm)
			depth := map[int]string{0: "15 bit", 1: "16 bit", 2: "24 bit", 3: "32 bit"}[r.PixelMode]
			if depth == "" {
				depth = "pixel mode " + pm
			}
			r.VideoName = fmt.Sprintf("%dx%d, %s colour", r.Width, r.Height, depth)
		case "m":
			free, arena, _ := strings.Cut(v, "/")
			r.FreeKB, _ = strconv.Atoi(free)
			r.ArenaKB, _ = strconv.Atoi(arena)
			if r.FreeKB == 0 && r.Reason != 3 && r.Reason != 4 {
				r.MemoryText = fmt.Sprintf("%d KB heap (free space is not counted after a stop)", r.ArenaKB)
			} else {
				r.MemoryText = fmt.Sprintf("%d KB free of %d KB", r.FreeKB, r.ArenaKB)
			}
		case "u":
			r.Uptime = n
		case "n":
			r.Games = n
		}
	}
}

func (r *Report) parseDevices(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	for _, seg := range strings.Split(s, ",") {
		m := devEntry.FindStringSubmatch(seg)
		if m == nil {
			// a product name with a comma in it: glue it back to the previous device
			if n := len(r.Devices); n > 0 {
				r.Devices[n-1].Name += "," + seg
			}
			continue
		}
		name := strings.TrimSpace(m[2])
		r.Devices = append(r.Devices, ReportDevice{Port: m[1], Name: name, Kind: deviceKind(name)})
	}
}

func deviceKind(name string) string {
	l := strings.ToLower(name)
	switch {
	case strings.Contains(l, "visual memory"):
		return "memory card"
	case strings.Contains(l, "controller"):
		return "controller"
	case strings.Contains(l, "puru puru"), strings.Contains(l, "vibration"):
		return "rumble pack"
	case strings.Contains(l, "keyboard"):
		return "keyboard"
	case strings.Contains(l, "mouse"):
		return "mouse"
	case strings.Contains(l, "microphone"):
		return "microphone"
	case strings.Contains(l, "arcade"):
		return "arcade stick"
	case strings.Contains(l, "fishing"):
		return "fishing controller"
	case strings.Contains(l, "gun"):
		return "light gun"
	}
	return name
}

func (r *Report) parseWarnings(s string) {
	if strings.TrimSpace(s) == "" {
		return
	}
	table := warnCodes()
	for _, seg := range strings.Split(s, ",") {
		code, cnt, ok := strings.Cut(strings.TrimSpace(seg), "x")
		if !ok || !strings.HasPrefix(code, "W") {
			r.Notes = append(r.Notes, "A warning counter the decoder could not read was skipped: "+seg)
			continue
		}
		n, _ := strconv.Atoi(cnt)
		w := ReportWarning{Code: code, Count: n}
		if wc, found := table[code]; found {
			w.Key, w.Text = wc.Key, wc.Text
		} else {
			w.Key, w.Text = "unknown", "A warning this Card Manager does not know (a newer SWIRL may have added it)"
		}
		r.Warnings = append(r.Warnings, w)
	}
}

func (r *Report) parseCrash(s string) {
	f := strings.Fields(s)
	if len(f) < 2 {
		r.Notes = append(r.Notes, "The crash address line was too short to read.")
		return
	}
	c := &ReportCrash{PC: ReportAddr{Addr: f[0]}, PR: ReportAddr{Addr: f[1]}, Stack: []ReportAddr{}}
	for _, a := range f[2:] {
		c.Stack = append(c.Stack, ReportAddr{Addr: a})
	}
	r.Crash = c
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// summary is the plain words paragraph the page shows first.
func (r *Report) summary() string {
	var s []string
	who := "SWIRL"
	if r.Version != "" {
		who += " " + r.Version
	}
	if r.Build != "" {
		b := "build " + r.Build
		if r.Dirty {
			b += " with uncommitted changes"
		}
		who += " (" + b + ")"
	}
	when := ""
	if r.Uptime > 0 {
		when = fmt.Sprintf(" %d s after power on", r.Uptime)
	}
	switch r.Reason {
	case 1:
		s = append(s, who+" crashed"+when+".")
	case 2:
		s = append(s, who+" stopped making progress"+when+".")
	case 3:
		s = append(s, who+" wrote this report when you opened System > Diagnostics"+when+".")
	case 4:
		s = append(s, who+" wrote its boot log"+when+" because X was held at power on.")
	case 5:
		s = append(s, who+" failed an internal check"+when+".")
	case 6:
		s = append(s, who+" stopped itself"+when+".")
	case 7:
		s = append(s, who+" could not start a game"+when+": the GDEMU did not answer and the menu disc did not come back.")
	default:
		s = append(s, who+" wrote this report"+when+".")
	}
	if r.LastStep != "" {
		s = append(s, "The last thing it did: "+r.LastStep+".")
	}
	if r.Crash != nil {
		where := r.Crash.PC.Addr
		if r.Crash.PC.Func != "" {
			where = fmt.Sprintf("%s (%s+%d)", r.Crash.PC.Func, r.Crash.PC.Addr, r.Crash.PC.Offset)
		}
		s = append(s, "It was at "+where+".")
	}
	var con []string
	if r.RegionName != "" {
		con = append(con, r.RegionName+" console")
	}
	if r.CableName != "" {
		con = append(con, r.CableName+" cable")
	}
	if r.VideoName != "" {
		con = append(con, r.VideoName)
	}
	if r.Games > 0 || r.Uptime > 0 {
		con = append(con, plural(r.Games, "game", "games"))
	}
	if len(con) > 0 {
		s = append(s, strings.Join(con, ", ")+".")
	}
	if len(r.Devices) > 0 {
		var d []string
		for _, x := range r.Devices {
			d = append(d, x.Kind+" on "+x.Port)
		}
		s = append(s, "Plugged in: "+strings.Join(d, ", ")+".")
	}
	if len(r.Warnings) > 0 {
		var w []string
		for _, x := range r.Warnings {
			times := "once"
			if x.Count != 1 {
				times = fmt.Sprintf("%d times", x.Count)
			}
			w = append(w, fmt.Sprintf("%s (%s, %s)", x.Text, x.Code, times))
		}
		s = append(s, plural(len(r.Warnings), "warning", "warnings")+" since power on: "+strings.Join(w, "; ")+".")
	} else if r.Reason == 3 || r.Reason == 4 {
		s = append(s, "No warnings since power on.")
	}
	if len(r.Missing) > 0 {
		s = append(s, fmt.Sprintf("%s of %d not scanned, so part of the report is missing.", plural(len(r.Missing), "part", "parts"), r.Parts))
	}
	return strings.Join(s, " ")
}

// ---------- symbols ----------

type elfSym struct {
	addr uint64
	size uint64
	name string
}

// symbolTable is the function table of a menu ELF, enough for "function+offset" (no DWARF needed).
type symbolTable struct {
	path   string
	syms   []elfSym
	rodata []byte // to check the build id the file was built with
}

func loadSymbols(path string) (*symbolTable, error) {
	f, err := elf.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	syms, err := f.Symbols()
	if err != nil {
		return nil, err
	}
	t := &symbolTable{path: path}
	for _, s := range syms {
		if elf.ST_TYPE(s.Info) != elf.STT_FUNC || s.Value == 0 || s.Name == "" {
			continue
		}
		t.syms = append(t.syms, elfSym{addr: s.Value, size: s.Size, name: strings.TrimPrefix(s.Name, "_")})
	}
	sort.Slice(t.syms, func(i, j int) bool { return t.syms[i].addr < t.syms[j].addr })
	if sec := f.Section(".rodata"); sec != nil {
		t.rodata, _ = sec.Data()
	}
	return t, nil
}

// hasBuild says whether the file holds the build id string, so its addresses match the report's.
func (t *symbolTable) hasBuild(build string) bool {
	if build == "" || t.rodata == nil {
		return false
	}
	// the string is "<hash>" or "<hash>-dirty", NUL terminated, as build.sh passed it
	for at, rest := 0, t.rodata; ; {
		i := bytes.Index(rest, []byte(build))
		if i < 0 {
			return false
		}
		at += i
		after := t.rodata[at+len(build):]
		if (at == 0 || t.rodata[at-1] == 0) && (bytes.HasPrefix(after, []byte{0}) || bytes.HasPrefix(after, []byte("-dirty\x00"))) {
			return true
		}
		at++
		rest = t.rodata[at:]
	}
}

// lookup finds the function an address is in. The SH4 cached and uncached views of the same code (0x8c and
// 0xac) both map to the symbols' 0x8c addresses.
func (t *symbolTable) lookup(addr uint64) (string, int, bool) {
	if addr>>29 == 5 { // 0xa0000000 to 0xbfffffff, the uncached window
		addr = addr&0x1fffffff | 0x80000000
	}
	i := sort.Search(len(t.syms), func(i int) bool { return t.syms[i].addr > addr }) - 1
	if i < 0 {
		return "", 0, false
	}
	s := t.syms[i]
	end := s.addr + s.size
	if s.size == 0 && i+1 < len(t.syms) {
		end = t.syms[i+1].addr
	}
	if addr >= end && s.size != 0 {
		return "", 0, false
	}
	return s.name, int(addr - s.addr), true
}

func (t *symbolTable) resolve(a *ReportAddr) {
	if !hexAddr.MatchString(a.Addr) {
		return
	}
	v, err := strconv.ParseUint(a.Addr, 16, 32)
	if err != nil {
		return
	}
	if name, off, ok := t.lookup(v); ok {
		a.Func, a.Offset = name, off
	}
}

// symbolsOverride is set by tests to point at a symbols file.
var symbolsOverride string

// symbolFiles lists the places a symbols file for the report's build may be, most likely first.
func symbolFiles(version string) []string {
	var dirs []string
	if symbolsOverride != "" {
		return []string{symbolsOverride}
	}
	dirs = append(dirs, filepath.Join(appDataDir(), "symbols"))
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		dirs = append(dirs, wd, filepath.Join(wd, "..", ".."))
	}
	var out []string
	for _, d := range dirs {
		if version != "" {
			out = append(out, filepath.Join(d, "SWIRL-"+version+"-symbols.elf"))
		}
		out = append(out, filepath.Join(d, "themeMenu.elf"))
		if version != "" {
			// anything the user dropped on the Report page
			m, _ := filepath.Glob(filepath.Join(d, "SWIRL-*-symbols.elf"))
			out = append(out, m...)
		}
	}
	return out
}

// symbolicate fills in function names for the x= addresses when a symbols file for the build is at hand.
func (r *Report) symbolicate() {
	if r.Crash == nil {
		return
	}
	var fallback *symbolTable
	seen := map[string]bool{}
	for _, p := range symbolFiles(r.Version) {
		if seen[p] {
			continue
		}
		seen[p] = true
		if _, err := os.Stat(p); err != nil {
			continue
		}
		t, err := loadSymbols(p)
		if err != nil || len(t.syms) == 0 {
			continue
		}
		if t.hasBuild(r.Build) {
			r.applySymbols(t, "")
			return
		}
		if fallback == nil {
			fallback = t
		}
	}
	if fallback != nil {
		r.applySymbols(fallback, "This symbols file was built from another commit than the report's build "+r.Build+", so the function names are a guess.")
		return
	}
	r.SymbolsNote = "No symbols file for this build was found, so the addresses are shown as numbers. Drop the build's themeMenu.elf or SWIRL-" + r.Version + "-symbols.elf on this page to name them."
}

func (r *Report) applySymbols(t *symbolTable, note string) {
	r.Symbols = t.path
	r.SymbolsNote = note
	t.resolve(&r.Crash.PC)
	t.resolve(&r.Crash.PR)
	for i := range r.Crash.Stack {
		t.resolve(&r.Crash.Stack[i])
	}
}

// ---------- the GitHub issue text ----------

// IssueBody is the markdown the "Copy for GitHub" button puts on the clipboard. Nothing is sent anywhere by
// Card Manager; the user pastes it into a new issue.
func IssueBody(r *Report, cmVersion, cardSummary string) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	w("## What happened")
	w("")
	w("<!-- Say what you were doing and what you expected. -->")
	w("")
	w("## SWIRL report")
	w("")
	w("%s", r.Summary)
	w("")
	w("| | |")
	w("|---|---|")
	if r.Version != "" {
		w("| SWIRL | %s |", r.Version)
	}
	if r.Build != "" {
		d := ""
		if r.Dirty {
			d = " (uncommitted changes)"
		}
		w("| Build | %s%s |", r.Build, d)
	}
	if r.ReasonName != "" {
		w("| Reason | R%d %s |", r.Reason, r.ReasonName)
	}
	if r.RegionName != "" {
		w("| Console | %s, BIOS %s, %s cable, %s |", r.RegionName, r.BIOS, r.CableName, r.VideoName)
	}
	if r.MemoryText != "" {
		w("| Memory | %s |", r.MemoryText)
	}
	if r.Uptime > 0 {
		w("| Uptime | %d s |", r.Uptime)
	}
	w("| Games | %d |", r.Games)
	if len(r.Devices) > 0 {
		var d []string
		for _, x := range r.Devices {
			d = append(d, x.Port+" "+x.Name)
		}
		w("| Devices | %s |", strings.Join(d, ", "))
	}
	w("| Card Manager | %s |", cmVersion)
	if len(r.Warnings) > 0 {
		w("")
		w("Warnings:")
		w("")
		for _, x := range r.Warnings {
			line := fmt.Sprintf("- %s %s x%d", x.Code, x.Text, x.Count)
			if x.Detail != "" {
				line += ", first: " + x.Detail
			}
			w("%s", line)
		}
	}
	if r.Crash != nil {
		w("")
		w("Where it stopped:")
		w("")
		w("- pc %s", addrText(r.Crash.PC))
		w("- pr %s", addrText(r.Crash.PR))
		for _, a := range r.Crash.Stack {
			w("- stack %s", addrText(a))
		}
		if r.SymbolsNote != "" {
			w("")
			w("%s", r.SymbolsNote)
		}
	}
	if len(r.Notes) > 0 {
		w("")
		for _, n := range r.Notes {
			w("- %s", n)
		}
	}
	if cardSummary != "" {
		w("")
		w("## Card")
		w("")
		w("%s", strings.TrimSpace(cardSummary))
	}
	w("")
	w("<details><summary>Raw report</summary>")
	w("")
	w("```")
	w("%s", strings.TrimSpace(r.Raw))
	w("```")
	w("")
	w("</details>")
	return b.String()
}

func addrText(a ReportAddr) string {
	if a.Func != "" {
		return fmt.Sprintf("%s (%s+%d)", a.Addr, a.Func, a.Offset)
	}
	return a.Addr
}

// saveSymbols keeps a symbols file the user dropped on the Report page, under the app's data folder, so the
// next report from that build is named without asking again.
func saveSymbols(name string, data []byte) (string, error) {
	if !bytes.HasPrefix(data, []byte{0x7f, 'E', 'L', 'F'}) {
		return "", fmt.Errorf("%s is not an ELF file", name)
	}
	base := filepath.Base(name)
	if base == "" || base == "." || strings.ContainsAny(base, "/\\") {
		base = "themeMenu.elf"
	}
	dir := filepath.Join(appDataDir(), "symbols")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, base)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", err
	}
	if _, err := loadSymbols(p); err != nil {
		os.Remove(p)
		return "", fmt.Errorf("%s could not be read as a symbols file: %v", base, err)
	}
	return p, nil
}
