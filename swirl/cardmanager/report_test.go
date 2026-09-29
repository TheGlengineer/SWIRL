package main

// The report decoder (report.go). The sample texts are what zbar read from the Flycast harness's QR
// screens (emu/runs/ws_d_t6_diag, ws_d_t6_badini, ws_d_t6_crash), trimmed.

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const diagPart1 = `SWIRL 2.14 1/3
SWIRL 2.14.0-preview.3 a747bc2-dirty R3
b=????? r=2 c=0 v=640x480/2 m=106/1317 u=23 n=25
d=A0:Dreamcast Controller,A1:Visual Memory,A2:Visual Memory
w=W04x1,W02x2
t=
156 controller scan done in 71 ms
160 SWIRL 2.14.0-preview.3 starting
2082 W04 OPENMENU.INI: bad line 85 skipped
2091 db_load_DAT()
2372 SWIRL: pictures
`

const diagPart2 = `SWIRL 2.14 2/3
7763 SWIRL: graphics
7836 SWIRL: VMU screen
9148 start up done
10857 first picture drawn
15962 save: start (SWIRL.DAT yes, settings no)
`

const diagPart3 = `SWIRL 2.14 3/3
17201 OPENMENU.CFG: none found, writing one
17925 save: SWIRL.DAT done (0)
22615 memory ok after save (42 KB free of 1317)
23006 report shown: Diagnostics
`

const crashReport = `SWIRL 2.14 1/1
SWIRL 2.14.0-preview.3 a747bc2-dirty R1
b=????? r=2 c=0 v=640x480/1 m=0/1277 u=9 n=25
d=A0:Dreamcast Controller,A1:Visual Memory,A2:Visual Memory
w=
x=8c000002 8c032720 8c0323f0 8c010100
t=
156 controller scan done in 71 ms
160 SWIRL 2.14.0-preview.3 starting
1271 graphics chip ready
9148 crash 180 pc=8c000002 pr=8c032720
`

func TestReportPartsInAnyOrder(t *testing.T) {
	r, err := ParseReport(diagPart3 + "\n" + diagPart1 + diagPart2)
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "2.14.0-preview.3" || r.Build != "a747bc2" || !r.Dirty || r.Reason != 3 || r.ReasonName != "diagnostics" {
		t.Fatalf("header: %+v", r)
	}
	if r.Parts != 3 || r.Found != 3 || len(r.Missing) != 0 {
		t.Fatalf("parts %d found %d missing %v", r.Parts, r.Found, r.Missing)
	}
	if r.RegionName != "Europe" || r.CableName != "VGA" || r.Width != 640 || r.Height != 480 || r.PixelMode != 2 || r.FreeKB != 106 || r.ArenaKB != 1317 || r.Uptime != 23 || r.Games != 25 {
		t.Fatalf("fields: %+v", r)
	}
	if len(r.Devices) != 3 || r.Devices[1].Port != "A1" || r.Devices[1].Name != "Visual Memory" || r.Devices[1].Kind != "memory card" || r.Devices[0].Kind != "controller" {
		t.Fatalf("devices: %+v", r.Devices)
	}
	if len(r.Warnings) != 2 || r.Warnings[0].Code != "W04" || r.Warnings[0].Key != "ini-line" || r.Warnings[0].Count != 1 || r.Warnings[0].Text != "OPENMENU.INI has lines SWIRL could not use" || r.Warnings[0].Detail != "OPENMENU.INI: bad line 85 skipped" {
		t.Fatalf("warnings: %+v", r.Warnings)
	}
	if r.Warnings[1].Code != "W02" || r.Warnings[1].Count != 2 || r.Warnings[1].Detail != "" {
		t.Fatalf("warning 2: %+v", r.Warnings[1])
	}
	if r.Crash != nil {
		t.Fatal("no crash line expected")
	}
	// the trace is the three parts in order
	if len(r.Trace) != 14 || r.Trace[0].MS != 156 || r.Trace[5].Text != "SWIRL: graphics" || r.Trace[13].Text != "report shown: Diagnostics" {
		t.Fatalf("trace: %+v", r.Trace)
	}
	if r.Trace[2].Code != "W04" {
		t.Fatalf("warning line not marked: %+v", r.Trace[2])
	}
	if r.LastStep != "memory ok after save (42 KB free of 1317)" {
		t.Fatalf("last step %q", r.LastStep)
	}
	if !strings.Contains(r.Summary, "System > Diagnostics") || !strings.Contains(r.Summary, "OPENMENU.INI has lines SWIRL could not use (W04, once)") || !strings.Contains(r.Summary, "memory card on A1") {
		t.Fatalf("summary: %s", r.Summary)
	}
	if strings.Contains(r.Raw, "1/3") || !strings.HasPrefix(r.Raw, "SWIRL 2.14.0-preview.3") {
		t.Fatalf("raw: %q", r.Raw)
	}
}

func TestReportMissingPart(t *testing.T) {
	r, err := ParseReport(diagPart1 + diagPart3)
	if err != nil {
		t.Fatal(err)
	}
	if r.Found != 2 || len(r.Missing) != 1 || r.Missing[0] != 2 {
		t.Fatalf("missing %v found %d", r.Missing, r.Found)
	}
	found := false
	for _, n := range r.Notes {
		if strings.Contains(n, "Part 2 of 3 is missing") {
			found = true
		}
	}
	if !found {
		t.Fatalf("notes: %v", r.Notes)
	}
	// the gap is marked in the trace where part 2 would have been
	if r.Trace[5].MS != -1 || !strings.Contains(r.Trace[5].Text, "part 2 of 3") || r.Trace[6].MS != 17201 {
		t.Fatalf("trace: %+v", r.Trace[4:7])
	}
	if !strings.Contains(r.Summary, "1 part of 3 not scanned") {
		t.Fatalf("summary: %s", r.Summary)
	}
	// the first part missing: no header, the trace still comes through and the version is taken from it
	r, err = ParseReport(diagPart2 + diagPart3)
	if err != nil {
		t.Fatal(err)
	}
	if r.Version != "" || r.Reason != 0 || len(r.Trace) < 9 || r.Trace[1].Text != "SWIRL: graphics" {
		t.Fatalf("without part 1: %+v", r)
	}
	if !strings.Contains(strings.Join(r.Notes, " "), "first line") {
		t.Fatalf("notes: %v", r.Notes)
	}
	// a duplicate scan of one part is used once
	r, _ = ParseReport(diagPart1 + diagPart2 + diagPart2 + diagPart3)
	if len(r.Trace) != 14 || !strings.Contains(strings.Join(r.Notes, " "), "more than once") {
		t.Fatalf("duplicate: %d lines, notes %v", len(r.Trace), r.Notes)
	}
}

func TestReportNoHeadersAndNoise(t *testing.T) {
	// a serial capture pasted without part headers, with a Reddit comment above it
	one := strings.Join(strings.Split(crashReport, "\n")[1:], "\n")
	r, err := ParseReport("Here is what my Dreamcast showed:\n\n" + one)
	if err != nil {
		t.Fatal(err)
	}
	if r.Parts != 0 || r.Reason != 1 || r.Crash == nil {
		t.Fatalf("%+v", r)
	}
	if _, err := ParseReport("hello world\nnothing here"); err == nil {
		t.Fatal("random text should not parse")
	}
	// Windows line ends
	if r, err := ParseReport(strings.ReplaceAll(diagPart1, "\n", "\r\n")); err != nil || r.Games != 25 {
		t.Fatalf("crlf: %v %+v", err, r)
	}
}

// fakeELF writes a small SH ELF with a symbol table and a .rodata, enough for loadSymbols.
func fakeELF(t *testing.T, path string, funcs []elfSym, rodata []byte) {
	t.Helper()
	var strtab bytes.Buffer
	strtab.WriteByte(0)
	var symtab bytes.Buffer
	symtab.Write(make([]byte, 16)) // the null symbol
	for _, f := range funcs {
		off := strtab.Len()
		strtab.WriteString("_" + f.name)
		strtab.WriteByte(0)
		var e [16]byte
		binary.LittleEndian.PutUint32(e[0:], uint32(off))
		binary.LittleEndian.PutUint32(e[4:], uint32(f.addr))
		binary.LittleEndian.PutUint32(e[8:], uint32(f.size))
		e[12] = 0x12 // global function
		binary.LittleEndian.PutUint16(e[14:], 1)
		symtab.Write(e[:])
	}
	shstr := []byte("\x00.rodata\x00.symtab\x00.strtab\x00.shstrtab\x00")
	// header, then the four data blobs, then the section headers
	var body bytes.Buffer
	type sec struct{ name, typ, link, info, entsize, off, size uint32 }
	secs := []sec{{}}
	add := func(name uint32, typ uint32, data []byte, link, info, entsize uint32) {
		secs = append(secs, sec{name: name, typ: typ, link: link, info: info, entsize: entsize, off: uint32(52 + body.Len()), size: uint32(len(data))})
		body.Write(data)
	}
	add(1, 1, rodata, 0, 0, 0)          // .rodata
	add(9, 2, symtab.Bytes(), 3, 1, 16) // .symtab, strings in section 3
	add(17, 3, strtab.Bytes(), 0, 0, 0) // .strtab
	add(25, 3, shstr, 0, 0, 0)          // .shstrtab
	shoff := uint32(52 + body.Len())
	var out bytes.Buffer
	ident := []byte{0x7f, 'E', 'L', 'F', 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	out.Write(ident)
	hdr := []uint16{2, 42}
	binary.Write(&out, binary.LittleEndian, hdr)
	binary.Write(&out, binary.LittleEndian, []uint32{1, 0x8c010000, 0, shoff, 0})
	binary.Write(&out, binary.LittleEndian, []uint16{52, 0, 0, 40, uint16(len(secs)), 4})
	out.Write(body.Bytes())
	for _, s := range secs {
		binary.Write(&out, binary.LittleEndian, []uint32{s.name, s.typ, 0, 0, s.off, s.size, s.link, s.info, 1, s.entsize})
	}
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReportSymbolicate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "SWIRL-2.14.0-preview.3-symbols.elf")
	fakeELF(t, p, []elfSym{
		{addr: 0x8c010100, size: 0x40, name: "draw_init"},
		{addr: 0x8c0323ec, size: 0x90, name: "__wrap_maple_wait_scan"},
		{addr: 0x8c032488, size: 0x400, name: "main"},
	}, []byte("\x00a747bc2-dirty\x00other\x00"))
	symbolsOverride = p
	defer func() { symbolsOverride = "" }()
	r, err := ParseReport(crashReport)
	if err != nil {
		t.Fatal(err)
	}
	if r.Crash == nil || r.Symbols != p || r.SymbolsNote != "" {
		t.Fatalf("crash %+v symbols %q note %q", r.Crash, r.Symbols, r.SymbolsNote)
	}
	if r.Crash.PC.Func != "" { // 8c000002 is below every function
		t.Fatalf("pc named: %+v", r.Crash.PC)
	}
	if r.Crash.PR.Func != "main" || r.Crash.PR.Offset != 0x298 {
		t.Fatalf("pr: %+v", r.Crash.PR)
	}
	if len(r.Crash.Stack) != 2 || r.Crash.Stack[0].Func != "__wrap_maple_wait_scan" || r.Crash.Stack[0].Offset != 4 || r.Crash.Stack[1].Func != "draw_init" || r.Crash.Stack[1].Offset != 0 {
		t.Fatalf("stack: %+v", r.Crash.Stack)
	}
	// the uncached view of the same address names the same function
	tbl, _ := loadSymbols(p)
	if n, off, ok := tbl.lookup(0xac032490); !ok || n != "main" || off != 8 {
		t.Fatalf("uncached lookup: %s %d %v", n, off, ok)
	}
	if _, _, ok := tbl.lookup(0x8c010140); ok { // one past draw_init's end
		t.Fatal("address past a function's end should not resolve")
	}
	// another build's symbols are used with a note
	q := filepath.Join(dir, "themeMenu.elf")
	fakeELF(t, q, []elfSym{{addr: 0x8c032488, size: 0x400, name: "main"}}, []byte("\x00db26f01\x00"))
	symbolsOverride = q
	r, _ = ParseReport(crashReport)
	if r.Crash.PR.Func != "main" || !strings.Contains(r.SymbolsNote, "another commit") {
		t.Fatalf("other build: %+v note %q", r.Crash.PR, r.SymbolsNote)
	}
	// no file at all: a note that says what to drop on the page
	symbolsOverride = filepath.Join(dir, "none.elf")
	r, _ = ParseReport(crashReport)
	if r.Symbols != "" || !strings.Contains(r.SymbolsNote, "SWIRL-2.14.0-preview.3-symbols.elf") {
		t.Fatalf("no symbols: %q %q", r.Symbols, r.SymbolsNote)
	}
	if !strings.Contains(r.Summary, "It was at 8c000002.") {
		t.Fatalf("summary: %s", r.Summary)
	}
}

func TestReportRealSymbols(t *testing.T) {
	// the menu's own ELF at the repo root, when this checkout has one (it is not committed)
	p := filepath.Join("..", "..", "themeMenu.elf")
	if _, err := os.Stat(p); err != nil {
		t.Skip("no themeMenu.elf at the repo root")
	}
	tbl, err := loadSymbols(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.syms) < 100 {
		t.Fatalf("only %d functions", len(tbl.syms))
	}
	var mainAddr uint64
	for _, s := range tbl.syms {
		if s.name == "main" {
			mainAddr = s.addr
		}
	}
	if mainAddr == 0 {
		t.Fatal("no main")
	}
	if n, off, ok := tbl.lookup(mainAddr + 6); !ok || n != "main" || off != 6 {
		t.Fatalf("main+6: %s %d %v", n, off, ok)
	}
	// the build id build.sh baked in is found in .rodata (the file may be dirty, so try both spellings)
	if out, err := exec.Command("git", "-C", filepath.Join("..", ".."), "rev-parse", "--short", "HEAD").Output(); err == nil {
		if h := strings.TrimSpace(string(out)); h != "" && !tbl.hasBuild(h) {
			t.Logf("themeMenu.elf was not built from HEAD %s (fine after a rebuild elsewhere)", h)
		}
	}
	if tbl.hasBuild("0000000") {
		t.Fatal("a made up build id matched")
	}
}

func TestReportIssueBody(t *testing.T) {
	symbolsOverride = filepath.Join(t.TempDir(), "none.elf")
	defer func() { symbolsOverride = "" }()
	r, _ := ParseReport(crashReport)
	body := IssueBody(r, "2.14.0-preview.4", "SWIRL 2.14 on the card, 25 games")
	for _, want := range []string{"## What happened", "| SWIRL | 2.14.0-preview.3 |", "| Build | a747bc2 (uncommitted changes) |", "| Reason | R1 crash |", "| Card Manager | 2.14.0-preview.4 |", "- pc 8c000002", "## Card\n\nSWIRL 2.14 on the card, 25 games", "```\nSWIRL 2.14.0-preview.3 a747bc2-dirty R1\n", "x=8c000002 8c032720 8c0323f0 8c010100\n"} {
		if !strings.Contains(body, want) {
			t.Fatalf("issue body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "1/1") {
		t.Fatal("part headers should not be in the raw block")
	}
	r, _ = ParseReport(diagPart1 + diagPart2 + diagPart3)
	body = IssueBody(r, "x", "")
	if !strings.Contains(body, "- W04 OPENMENU.INI has lines SWIRL could not use x1, first: OPENMENU.INI: bad line 85 skipped") || strings.Contains(body, "## Card") {
		t.Fatalf("diag body:\n%s", body)
	}
}

func TestReportCodesMatchMenu(t *testing.T) {
	// assets/codes.tsv is a copy of swirl/diag/codes.tsv, the table the menu is generated from
	want, err := os.ReadFile(filepath.Join("..", "diag", "codes.tsv"))
	if err != nil {
		t.Skip("swirl/diag/codes.tsv not next to this checkout")
	}
	if string(want) != codesTSV {
		t.Fatal("assets/codes.tsv differs from swirl/diag/codes.tsv: copy it over")
	}
	codes := warnCodes()
	if len(codes) < 19 || codes["W19"].Key != "disc-read" || codes["W01"].Text != "A save to the VMU failed" {
		t.Fatalf("codes: %d, %+v", len(codes), codes["W19"])
	}
}

func TestSaveSymbols(t *testing.T) {
	old := dbDirOverride
	dbDirOverride = filepath.Join(t.TempDir(), "db")
	defer func() { dbDirOverride = old }()
	if _, err := saveSymbols("x.elf", []byte("not an elf")); err == nil {
		t.Fatal("junk accepted")
	}
	src := filepath.Join(t.TempDir(), "SWIRL-9.9.9-symbols.elf")
	fakeELF(t, src, []elfSym{{addr: 0x8c010000, size: 4, name: "f"}}, nil)
	data, _ := os.ReadFile(src)
	p, err := saveSymbols("../../SWIRL-9.9.9-symbols.elf", data)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "SWIRL-9.9.9-symbols.elf" || !strings.HasPrefix(p, filepath.Join(appDataDir(), "symbols")) {
		t.Fatalf("saved at %s", p)
	}
}
