package main

// Suite K: card fault injection. Every card write goes through cardfs; these tests swap in faultFS, which
// fails a chosen operation, and then check that the card is still usable. A row with fixed false only
// logs what it found, so the suite stays green until the commit that fixes the failure flips the row.

import (
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// ---------- faultFS ----------

type fault struct {
	op     string // Rename, Create, Write, MkdirAll, Remove, RemoveAll, SyncFile, SyncDir
	path   string // a substring of the path (of either path for Rename); empty matches any path
	exact  string // or the exact path
	n      int    // the nth matching call fails, counting from 1
	sticky bool   // every matching call from the nth on fails
	err    error  // syscall.EACCES, syscall.ENOSPC or syscall.EBUSY
	panic  bool   // stop dead instead, like a pulled card or a power cut
	seen   int
}

type faultFS struct {
	realFS
	mu     sync.Mutex
	faults []*fault
	fired  int
}

func (f *faultFS) check(op string, paths ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, x := range f.faults {
		if x.op != op {
			continue
		}
		match := x.path == "" && x.exact == ""
		for _, p := range paths {
			if (x.path != "" && strings.Contains(p, x.path)) || (x.exact != "" && filepath.Clean(p) == filepath.Clean(x.exact)) {
				match = true
			}
		}
		if !match {
			continue
		}
		x.seen++
		if x.seen == x.n || (x.sticky && x.seen > x.n) {
			f.fired++
			if x.panic {
				panic("power lost")
			}
			return x.err
		}
	}
	return nil
}

func (f *faultFS) Rename(o, n string) error {
	if err := f.check("Rename", o, n); err != nil {
		return &os.LinkError{Op: "rename", Old: o, New: n, Err: err}
	}
	return f.realFS.Rename(o, n)
}

func (f *faultFS) Create(name string) (io.WriteCloser, error) {
	if err := f.check("Create", name); err != nil {
		return nil, &os.PathError{Op: "open", Path: name, Err: err}
	}
	w, err := f.realFS.Create(name)
	if err != nil {
		return nil, err
	}
	return &faultWriter{w, f, name}, nil
}

func (f *faultFS) MkdirAll(p string, perm os.FileMode) error {
	if err := f.check("MkdirAll", p); err != nil {
		return &os.PathError{Op: "mkdir", Path: p, Err: err}
	}
	return f.realFS.MkdirAll(p, perm)
}

func (f *faultFS) Remove(p string) error {
	if err := f.check("Remove", p); err != nil {
		return &os.PathError{Op: "remove", Path: p, Err: err}
	}
	return f.realFS.Remove(p)
}

func (f *faultFS) RemoveAll(p string) error {
	if err := f.check("RemoveAll", p); err != nil {
		return &os.PathError{Op: "unlinkat", Path: p, Err: err}
	}
	return f.realFS.RemoveAll(p)
}

func (f *faultFS) SyncFile(w io.WriteCloser) error {
	fw, ok := w.(*faultWriter)
	if !ok {
		return f.realFS.SyncFile(w)
	}
	if err := f.check("SyncFile", fw.name); err != nil {
		return &os.PathError{Op: "sync", Path: fw.name, Err: err}
	}
	return f.realFS.SyncFile(fw.WriteCloser)
}

func (f *faultFS) SyncDir(dir string) error {
	if err := f.check("SyncDir", dir); err != nil {
		return &os.PathError{Op: "sync", Path: dir, Err: err}
	}
	return f.realFS.SyncDir(dir)
}

type faultWriter struct {
	io.WriteCloser
	fs   *faultFS
	name string
}

func (w *faultWriter) Write(b []byte) (int, error) {
	if err := w.fs.check("Write", w.name); err != nil {
		return 0, &os.PathError{Op: "write", Path: w.name, Err: err}
	}
	return w.WriteCloser.Write(b)
}

// useFaults installs a faultFS until the returned function is called (or the test ends).
func useFaults(t *testing.T, faults ...*fault) (*faultFS, func()) {
	ff := &faultFS{faults: faults}
	cardfs = ff
	done := func() { cardfs = realFS{} }
	t.Cleanup(done)
	return ff, done
}

// ---------- fixtures ----------

func quiet(string, ...any) {}

// makeMenuIn01 puts a menu disc in root/01 whose IP.BIN name is ipName ("openMenu", "GDMENU").
func makeMenuIn01(t *testing.T, root, ipName string) {
	w := t.TempDir()
	data, low := filepath.Join(w, "data"), filepath.Join(w, "low")
	os.MkdirAll(data, 0o755)
	os.MkdirAll(low, 0o755)
	os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), []byte(strings.Repeat("stock menu binary "+ipName, 300)), 0o644)
	os.WriteFile(filepath.Join(data, "OPENMENU.INI"), []byte("[OPENMENU]\r\nnum_items=1\r\n"), 0o644)
	os.WriteFile(filepath.Join(data, "MARKER.TXT"), []byte("a file only the original menu has"), 0o644)
	os.WriteFile(filepath.Join(low, "OPENMENU.INI"), []byte("[OPENMENU]\r\nnum_items=1\r\n"), 0o644)
	os.MkdirAll(filepath.Join(root, "01"), 0o755)
	if err := buildMenuDisc(data, low, filepath.Join(root, "01"), ipSector(ipName, "MENU-001")); err != nil {
		t.Fatal(err)
	}
}

// txnCard makes a card with games 02.. (CDI and GDI in turn, each with an edit, 03 with box art) and a menu:
// "swirl" (built by installSwirl), "openMenu" or "GDMENU".
func txnCard(t *testing.T, menu string, games int) string {
	root := t.TempDir()
	edits := loadEdits(root)
	for i := 0; i < games; i++ {
		f := fmt.Sprintf("%02d", i+2)
		title, serial := fmt.Sprintf("GAME %c", 'A'+i), fmt.Sprintf("T-%05dN", i+2)
		if i%2 == 0 {
			writeTestCDI(t, filepath.Join(root, f, "disc.cdi"), title, serial)
		} else {
			writeTestGDI(t, filepath.Join(root, f), title, serial)
		}
		edits.Games[f] = &GameEdit{Product: openMenuProduct(serial)}
	}
	if err := edits.save(); err != nil {
		t.Fatal(err)
	}
	if games >= 2 {
		os.WriteFile(artPath(root, "03", "box"), pngBytes(quadImage(64)), 0o644)
	}
	switch menu {
	case "swirl":
		if err := installSwirl(root, "", true, quiet); err != nil {
			t.Fatal(err)
		}
	case "":
	default:
		makeMenuIn01(t, root, menu)
	}
	return root
}

// ---------- the invariants ----------

type cardSnap struct {
	menu map[string]string // 01 file -> sha1
	want map[string]string // when set, the only acceptable new 01 (a restore); else a complete SWIRL build
}

func hashDir(dir string) map[string]string {
	out := map[string]string{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = fmt.Sprintf("%x", sha1.Sum(b))
		return nil
	})
	return out
}

func sameHashes(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func snapCard(root string) cardSnap { return cardSnap{menu: hashDir(filepath.Join(root, "01"))} }

func menuINI(root string) (string, error) {
	m, err := openMenuDisc(root)
	if err != nil {
		return "", err
	}
	defer m.Close()
	f, ok := m.files["OPENMENU.INI"]
	if !ok {
		return "", errors.New("no OPENMENU.INI on the menu disc")
	}
	tmp, err := os.MkdirTemp("", "ini")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := extractISOFile(m.d, f, filepath.Join(tmp, "i")); err != nil {
		return "", err
	}
	b, err := os.ReadFile(filepath.Join(tmp, "i"))
	return string(b), err
}

// iniMatchesFolders compares the menu's OPENMENU.INI with the game folders on the card.
func iniMatchesFolders(root string, c *Card) string {
	ini, err := menuINI(root)
	if err != nil {
		return "menu INI unreadable: " + err.Error()
	}
	want := map[string]string{}
	for _, g := range c.Games {
		want[fmt.Sprintf("%02d", g.Slot)] = g.Name
	}
	got := map[string]string{}
	for _, l := range strings.Split(ini, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), ".name="); ok && k != "01" {
			got[k] = v
		}
	}
	if fmt.Sprint(want) != fmt.Sprint(got) {
		return fmt.Sprintf("OPENMENU.INI lists %v but the folders hold %v", got, want)
	}
	return ""
}

var menuFiles = []string{"disc.gdi", "track01.iso", "track02.raw", "track03.iso", "track04.raw", "track05.iso"}

func leftovers(root string) []string {
	var out []string
	for _, n := range listDir(root) {
		if strings.HasPrefix(n, "_swirl_mv_") || strings.HasSuffix(n, ".part") {
			out = append(out, n)
		}
	}
	if fileExists(filepath.Join(root, editsDir, ".stage_01")) {
		out = append(out, editsDir+"/.stage_01")
	}
	return out
}

// assertCardSane returns what is wrong with the card after an operation that started from before.
func assertCardSane(t *testing.T, root string, before cardSnap) []string {
	t.Helper()
	var bad []string
	// a journal is allowed only if the next scan finishes or undoes it
	journals := []string{filepath.Join(root, editsDir, "renumber.json"), filepath.Join(root, editsDir, "RECOVER.json")}
	for _, j := range journals {
		if fileExists(j) {
			ScanCard(root)
			if fileExists(j) {
				bad = append(bad, "the next scan left "+filepath.Base(j)+" in place")
			}
		}
	}
	for _, l := range leftovers(root) {
		bad = append(bad, "left behind: "+l)
	}
	// 01: the old menu byte for byte, or the complete new one
	now := hashDir(filepath.Join(root, "01"))
	newMenu := false
	switch {
	case sameHashes(now, before.menu):
	case before.want != nil:
		if !sameHashes(now, before.want) {
			bad = append(bad, fmt.Sprintf("01 is neither the old menu nor the restored one: %v", sortedKeys(now)))
		}
	default:
		newMenu = true
		if fmt.Sprint(sortedKeys(now)) != fmt.Sprint(sortedCopy(menuFiles)) {
			bad = append(bad, fmt.Sprintf("01 is neither the old menu nor a complete new one: %v", sortedKeys(now)))
			newMenu = false
		} else if m, err := openMenuDisc(root); err != nil {
			bad = append(bad, "the new menu disc does not open: "+err.Error())
			newMenu = false
		} else {
			if f, ok := m.files["1ST_READ.BIN"]; !ok || f.Size != len(swirlBinary) {
				bad = append(bad, "the new menu disc has no complete 1ST_READ.BIN")
			}
			m.Close()
		}
	}
	// game folders 02..NN with no gaps
	nums := numberedFolders(root)
	for i, n := range nums {
		if n != i+2 {
			bad = append(bad, fmt.Sprintf("folder numbers are not contiguous: %v", nums))
			break
		}
	}
	// every .gdi only names files in its own folder, and they are there
	for _, dir := range append([]string{"01"}, folderNames(root, nums)...) {
		for _, n := range listDir(filepath.Join(root, dir)) {
			if !strings.EqualFold(filepath.Ext(n), ".gdi") || isJunk(n) {
				continue
			}
			tracks, err := parseGDI(filepath.Join(root, dir, n))
			if err != nil {
				bad = append(bad, fmt.Sprintf("%s/%s: %v", dir, n, err))
				continue
			}
			for _, tr := range tracks {
				if filepath.Dir(tr.File) != filepath.Join(root, dir) || !fileExists(tr.File) {
					bad = append(bad, fmt.Sprintf("%s/%s names %s, which is not in the folder", dir, n, filepath.Base(tr.File)))
				}
			}
		}
	}
	c, err := ScanCard(root)
	if err != nil {
		return append(bad, "scan: "+err.Error())
	}
	// games.json: every edit belongs to a folder that holds the game it was made for
	edits := loadEdits(root)
	for f, e := range edits.Games {
		g := findGame(c, f)
		if g == nil {
			bad = append(bad, "games.json has an edit for folder "+f+", which is not on the card")
		} else if g.Product != e.Product {
			bad = append(bad, fmt.Sprintf("games.json says folder %s holds %s, it holds %s", f, e.Product, g.Product))
		}
	}
	if newMenu {
		if s := iniMatchesFolders(root, c); s != "" {
			bad = append(bad, "new menu: "+s)
		}
	}
	return bad
}

func folderNames(root string, nums []int) []string {
	var out []string
	for _, n := range nums {
		out = append(out, folderName(root, n))
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// report fails a fixed row and only logs one that is still open.
func report(t *testing.T, fixed bool, bad []string) {
	t.Helper()
	if len(bad) == 0 {
		return
	}
	if fixed {
		t.Errorf("card not sane:\n  %s", strings.Join(bad, "\n  "))
		return
	}
	t.Logf("known failure (row not fixed yet):\n  %s", strings.Join(bad, "\n  "))
}

// ---------- operations ----------

func waitJobErr(t *testing.T) error {
	j := waitJobAny(t)
	if j.Error != "" {
		return errors.New(j.Error)
	}
	return nil
}

func opInstall(t *testing.T, root string) error { return InstallSwirl(root, "", quiet) }

func opRemove(folders ...string) func(*testing.T, string) error {
	return func(t *testing.T, root string) error {
		if err := StartRemoveGames(root, folders); err != nil {
			return err
		}
		return waitJobErr(t)
	}
}

func opReverse(t *testing.T, root string) error {
	var order []string
	nums := numberedFolders(root)
	for i := len(nums) - 1; i >= 0; i-- {
		order = append(order, folderName(root, nums[i]))
	}
	if err := StartReorder(root, order); err != nil {
		return err
	}
	return waitJobErr(t)
}

// ---------- the table ----------

type txnRow struct {
	name   string
	fixed  bool
	menu   string // what 01 holds before: swirl, openMenu, GDMENU
	games  int
	setup  func(t *testing.T, root string, s *cardSnap)
	faults func(root string) []*fault
	run    func(t *testing.T, root string) error
	extra  func(t *testing.T, root string, s cardSnap, err error) []string
}

func runTxnRows(t *testing.T, rows []txnRow) {
	for _, r := range rows {
		r := r
		t.Run(r.name, func(t *testing.T) {
			root := txnCard(t, r.menu, r.games)
			s := snapCard(root)
			if r.setup != nil {
				r.setup(t, root, &s)
			}
			var faults []*fault
			if r.faults != nil {
				faults = r.faults(root)
			}
			ff, done := useFaults(t, faults...)
			var err error
			func() {
				defer func() {
					if p := recover(); p != nil {
						err = fmt.Errorf("panic: %v", p)
						t.Errorf("panic: %v", p)
					}
				}()
				err = r.run(t, root)
			}()
			done()
			t.Logf("faults fired %d, result %v", ff.fired, err)
			bad := assertCardSane(t, root, s)
			if r.extra != nil {
				bad = append(bad, r.extra(t, root, s, err)...)
			}
			report(t, r.fixed, bad)
		})
	}
}

func renameAt(n int, err error) func(string) []*fault {
	return func(string) []*fault { return []*fault{{op: "Rename", n: n, err: err}} }
}

// original backups: how many SWIRL_BACKUP folders hold exactly the menu that was in 01 at the start
func backupsHolding(root string, menu map[string]string) int {
	n := 0
	for _, b := range listBackups(root) {
		if sameHashes(hashDir(filepath.Join(root, backupDir, b)), menu) {
			n++
		}
	}
	return n
}

func keepsOriginal(t *testing.T, root string, s cardSnap, err error) []string {
	if backupsHolding(root, s.menu) == 0 && !sameHashes(hashDir(filepath.Join(root, "01")), s.menu) {
		return []string{"the menu that was in 01 at the start is gone from the card"}
	}
	return nil
}

func TestCardFaultsInstall(t *testing.T) {
	var rows []txnRow
	// CM-2: a rename fails at step n (antivirus, indexer, an open file on Windows)
	for n := 1; n <= 6; n++ {
		rows = append(rows, txnRow{name: fmt.Sprintf("CM2_rename_fails_at_%d", n), fixed: true, menu: "openMenu", games: 3,
			faults: renameAt(n, syscall.EBUSY), run: opInstall, extra: keepsOriginal})
	}
	// CM-2: renames keep failing from step n, so the rollback fails too
	for n := 1; n <= 3; n++ {
		n := n
		rows = append(rows, txnRow{name: fmt.Sprintf("CM2_renames_fail_from_%d", n), fixed: true, menu: "openMenu", games: 3,
			faults: func(string) []*fault { return []*fault{{op: "Rename", n: n, sticky: true, err: syscall.EACCES}} },
			run:    opInstall, extra: keepsOriginal})
	}
	// disk full while the new menu is written
	for _, n := range []int{1, 4, 40} {
		n := n
		rows = append(rows, txnRow{name: fmt.Sprintf("disk_full_at_write_%d", n), fixed: true, menu: "openMenu", games: 3,
			faults: func(root string) []*fault {
				return []*fault{{op: "Write", path: root, n: n, sticky: true, err: syscall.ENOSPC}}
			},
			run: opInstall, extra: keepsOriginal})
	}
	rows = append(rows, txnRow{name: "disk_full_at_create", fixed: true, menu: "swirl", games: 3,
		faults: func(root string) []*fault {
			return []*fault{{op: "Create", path: filepath.Join(root, "01"), n: 3, sticky: true, err: syscall.ENOSPC}}
		}, run: opInstall})
	// L13: two installs in one second share one backup folder
	rows = append(rows, txnRow{name: "L13_two_installs_in_one_second", fixed: true, menu: "GDMENU", games: 2,
		run: func(t *testing.T, root string) error {
			time.Sleep(time.Until(time.Now().Truncate(time.Second).Add(time.Second)))
			if err := InstallSwirl(root, "", quiet); err != nil {
				return err
			}
			return InstallSwirl(root, "", quiet)
		}, extra: keepsOriginal})
	// CM-3: the menu that was on the card before SWIRL survives later rebuilds
	rows = append(rows, txnRow{name: "CM3_original_survives_rebuilds", fixed: true, menu: "openMenu", games: 2,
		run: func(t *testing.T, root string) error {
			for i := 0; i < 4; i++ {
				if err := InstallSwirl(root, "", quiet); err != nil {
					return err
				}
			}
			return nil
		}, extra: keepsOriginal})
	runTxnRows(t, rows)
}

func TestCardFaultsRestore(t *testing.T) {
	// the card had openMenu; SWIRL was installed, which kept openMenu as a backup
	origSetup := func(t *testing.T, root string, s *cardSnap) {
		if err := InstallSwirl(root, "", quiet); err != nil {
			t.Fatal(err)
		}
		bk := listBackups(root)
		if len(bk) != 1 {
			t.Fatalf("backups after install: %v", bk)
		}
		s.want = hashDir(filepath.Join(root, backupDir, bk[0]))
		s.menu = hashDir(filepath.Join(root, "01"))
	}
	restoreOnly := func(t *testing.T, root string) error { return RestoreBackup(root, listBackups(root)[0], quiet) }
	backupKept := func(t *testing.T, root string, s cardSnap, err error) []string {
		for _, b := range listBackups(root) {
			if sameHashes(hashDir(filepath.Join(root, backupDir, b)), s.want) {
				return nil
			}
		}
		return []string{"the backup that was restored is no longer on the card"}
	}
	rows := []txnRow{
		{name: "restore_keeps_the_backup", fixed: true, menu: "openMenu", games: 2, setup: origSetup, run: restoreOnly, extra: backupKept},
		// CM-2 and L11: a backup left half made by a failed install is restored over a good menu
		{name: "CM2_restore_of_a_half_backup", fixed: true, menu: "swirl", games: 2,
			setup: func(t *testing.T, root string, s *cardSnap) {
				half := filepath.Join(root, backupDir, "01_20000101_000000")
				os.MkdirAll(half, 0o755)
				for _, n := range []string{"disc.gdi", "track01.iso", "track02.raw"} {
					if err := copyFile(filepath.Join(root, "01", n), filepath.Join(half, n)); err != nil {
						t.Fatal(err)
					}
				}
				s.want = map[string]string{"refused": "refused"} // only the old menu is acceptable
			},
			run: func(t *testing.T, root string) error { return RestoreBackup(root, "01_20000101_000000", quiet) },
			extra: func(t *testing.T, root string, s cardSnap, err error) []string {
				if err == nil {
					return []string{"restoring a backup with missing tracks was not refused"}
				}
				return nil
			}},
	}
	for n := 1; n <= 6; n++ {
		rows = append(rows, txnRow{name: fmt.Sprintf("CM2_restore_rename_fails_at_%d", n), fixed: true, menu: "openMenu", games: 2,
			setup: origSetup, faults: renameAt(n, syscall.EBUSY), run: restoreOnly, extra: backupKept})
	}
	runTxnRows(t, rows)
}

func TestCardFaultsRenumber(t *testing.T) {
	var rows []txnRow
	// CM-5: one folder is busy while games are removed
	busy04 := func(root string) []*fault {
		return []*fault{{op: "Rename", exact: filepath.Join(root, "04"), n: 1, sticky: true, err: syscall.EBUSY}}
	}
	rows = append(rows,
		txnRow{name: "CM5_remove_02_with_04_busy", fixed: true, menu: "swirl", games: 5, faults: busy04, run: opRemove("02")},
		txnRow{name: "CM5_remove_03_04_with_04_busy", fixed: true, menu: "swirl", games: 4, faults: busy04, run: opRemove("03", "04")},
	)
	for n := 1; n <= 10; n++ {
		rows = append(rows, txnRow{name: fmt.Sprintf("CM5_remove_rename_fails_at_%d", n), fixed: true, menu: "swirl", games: 5,
			faults: renameAt(n, syscall.EBUSY), run: opRemove("03")})
	}
	for n := 1; n <= 8; n++ {
		rows = append(rows, txnRow{name: fmt.Sprintf("CM5_reorder_rename_fails_at_%d", n), fixed: true, menu: "swirl", games: 4,
			faults: renameAt(n, syscall.EACCES), run: opReverse})
	}
	// CM-5: remove duplicates with the extra copy busy
	rows = append(rows, txnRow{name: "CM5_dedupe_with_a_busy_copy", fixed: true, menu: "swirl", games: 3,
		setup: func(t *testing.T, root string, s *cardSnap) {
			writeTestCDI(t, filepath.Join(root, "05", "disc.cdi"), "GAME A", "T-00002N")
			writeTestCDI(t, filepath.Join(root, "06", "disc.cdi"), "GAME A", "T-00002N")
			if err := installSwirl(root, "", true, quiet); err != nil {
				t.Fatal(err)
			}
			*s = snapCard(root)
		},
		faults: func(root string) []*fault {
			return []*fault{{op: "Rename", exact: filepath.Join(root, "06"), n: 1, sticky: true, err: syscall.EBUSY}}
		},
		run: func(t *testing.T, root string) error { return RemoveDuplicates(root, quiet) }})
	runTxnRows(t, rows)
}

func TestCardFaultsAdd(t *testing.T) {
	src := t.TempDir()
	writeTestGDI(t, filepath.Join(src, "New GDI Game"), "NEW GDI GAME", "T-00077N")
	writeTestCDI(t, filepath.Join(src, "New CDI Game.cdi"), "NEW CDI GAME", "T-00078N")
	add := func(t *testing.T, root string) error {
		if err := StartAddGames(root, []string{filepath.Join(src, "New GDI Game"), filepath.Join(src, "New CDI Game.cdi")}, ""); err != nil {
			return err
		}
		return waitJobErr(t)
	}
	noNewFolders := func(t *testing.T, root string, s cardSnap, err error) []string {
		if err == nil {
			return nil
		}
		if nums := numberedFolders(root); len(nums) != 3 {
			return []string{fmt.Sprintf("the add failed but left folders %v", nums)}
		}
		return nil
	}
	var rows []txnRow
	// CM-9: the copy fails part way
	for _, n := range []int{1, 2, 3, 5} {
		n := n
		rows = append(rows, txnRow{name: fmt.Sprintf("CM9_add_write_fails_at_%d", n), fixed: true, menu: "swirl", games: 3,
			faults: func(root string) []*fault { return []*fault{{op: "Write", path: root, n: n, err: syscall.ENOSPC}} },
			run:    add, extra: noNewFolders})
	}
	runTxnRows(t, rows)
}

// CM-6: an install and a remove started at the same time
func TestCardConcurrentInstallRemove(t *testing.T) {
	const iters = 20
	fixed := true
	broken := 0
	for it := 0; it < iters; it++ {
		root := txnCard(t, "swirl", 5)
		s := snapCard(root)
		var errA, errB error
		done := make(chan struct{})
		go func() { // what /api/install does
			defer close(done)
			unlock, err := lockCard(root, "Update SWIRL")
			if err != nil {
				errA = err
				return
			}
			defer unlock()
			errA = InstallSwirl(root, "", quiet)
		}()
		time.Sleep(time.Duration(it) * 15 * time.Millisecond)
		if errB = StartRemoveGames(root, []string{"03"}); errB == nil {
			errB = waitJobErr(t)
		}
		<-done
		bad := assertCardSane(t, root, s)
		if c, err := ScanCard(root); err == nil {
			if x := iniMatchesFolders(root, c); x != "" {
				bad = append(bad, x)
			}
		}
		if len(bad) > 0 {
			broken++
			t.Logf("run %d: install %v, remove %v", it, errA, errB)
			report(t, fixed, bad)
		}
	}
	t.Logf("%d of %d runs left a broken card", broken, iters)
}

// A1: a second writer is turned away with a clear message; reading goes on.
func TestCardLock(t *testing.T) {
	old := cardLockWait
	cardLockWait = 50 * time.Millisecond
	defer func() { cardLockWait = old }()
	root := txnCard(t, "swirl", 3)
	unlock, err := lockCard(root, "Update SWIRL")
	if err != nil {
		t.Fatal(err)
	}
	if !anyCardBusy() {
		t.Error("busy() would let the app quit during a write")
	}
	// the same card under another spelling of its path
	if _, err := lockCard(root+string(filepath.Separator), "Names"); err == nil || err.Error() != "Another operation is running on this card. Wait for it to finish." {
		t.Fatalf("second writer: %v", err)
	}
	if err := StartRemoveGames(root, []string{"03"}); err != errCardBusy {
		t.Fatalf("job while the card is busy: %v", err)
	}
	if j := jobSnapshot(); j.Running {
		t.Fatal("a refused job left the job slot taken")
	}
	if err := startNewCard(NewCardRequest{Root: root}); err != errCardBusy {
		t.Fatalf("new card while the card is busy: %v", err)
	}
	if j := jobSnapshot(); j.Running {
		t.Fatal("a refused new card left the job slot taken")
	}
	if c, err := ScanCard(root); err != nil || len(c.Games) != 3 {
		t.Fatalf("scan while busy: %v", err)
	}
	unlock()
	unlock() // a second release is harmless
	if anyCardBusy() {
		t.Error("card still busy after release")
	}
	// a job holds the lock for its whole run
	release := make(chan struct{})
	if err := runJob(root, "Test job", "", func() error { <-release; return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := lockCard(root, "Update SWIRL"); err != errCardBusy {
		t.Fatalf("writer during a job: %v", err)
	}
	close(release)
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	u, err := lockCard(root, "Update SWIRL")
	if err != nil {
		t.Fatalf("lock after the job: %v", err)
	}
	u()
}

// L13: backup folders made in the same second get their own names
func TestUniqueBackupNames(t *testing.T) {
	root := t.TempDir()
	a := uniqueBackupPath(root, "01_")
	os.MkdirAll(a, 0o755)
	b := uniqueBackupPath(root, "01_")
	os.MkdirAll(b, 0o755)
	c := uniqueBackupPath(root, "01_")
	if !strings.HasPrefix(c, a) {
		t.Skip("the clock moved to the next second")
	}
	if b != a+"_2" || c != a+"_3" {
		t.Fatalf("names %s %s %s", a, b, c)
	}
}

func openMenuReaders() int {
	thumbMu.Lock()
	defer thumbMu.Unlock()
	n := 0
	for _, tc := range thumbCards {
		if tc.menu != nil {
			n++
		}
	}
	return n
}

// A2: the cover grid keeps the menu disc open; taking the card lock closes it and keeps it closed until the
// write is over. Linux renames open files, so only the Windows runner proves the rename itself succeeds.
func TestLockClosesMenuReaders(t *testing.T) {
	root := txnCard(t, "swirl", 3)
	GameThumb(root, "02", "T00002N", false)
	if openMenuReaders() == 0 {
		t.Fatal("the cover grid did not open the menu disc; the test proves nothing")
	}
	unlock, err := lockCard(root, "Update SWIRL")
	if err != nil {
		t.Fatal(err)
	}
	if n := openMenuReaders(); n != 0 {
		t.Fatalf("%d menu disc readers still open after the card lock was taken", n)
	}
	GameThumb(root, "04", "T00004N", true) // a cover asked for during the write
	if n := openMenuReaders(); n != 0 {
		t.Fatalf("a cover request opened the menu disc during a write")
	}
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	unlock()
	GameThumb(root, "04", "T00004N", true)
	if openMenuReaders() == 0 {
		t.Fatal("covers did not open the menu disc again after the write")
	}
}

// A3: backups never lose a menu. The original is pinned, restores copy, and an interrupted swap is undone.
func TestMenuBackups(t *testing.T) {
	root := txnCard(t, "openMenu", 2)
	orig := hashDir(filepath.Join(root, "01"))
	for i := 0; i < 5; i++ {
		if err := InstallSwirl(root, "", quiet); err != nil {
			t.Fatal(err)
		}
	}
	var original, auto []string
	for _, b := range listBackups(root) {
		switch {
		case strings.HasPrefix(b, "01_original_openmenu_"):
			original = append(original, b)
		case strings.HasPrefix(b, "01_2"):
			auto = append(auto, b)
		default:
			t.Errorf("unexpected backup %s", b)
		}
	}
	if len(original) != 1 || len(auto) != 3 {
		t.Fatalf("original %v, automatic %v", original, auto)
	}
	if !sameHashes(hashDir(filepath.Join(root, backupDir, original[0])), orig) {
		t.Fatal("the original backup does not hold the original menu")
	}
	// restore copies the backup: it is still there afterwards, and the replaced menu is kept too
	swirl := hashDir(filepath.Join(root, "01"))
	if err := RestoreBackup(root, original[0], quiet); err != nil {
		t.Fatal(err)
	}
	if !sameHashes(hashDir(filepath.Join(root, "01")), orig) {
		t.Fatal("01 is not the original menu after the restore")
	}
	if !sameHashes(hashDir(filepath.Join(root, backupDir, original[0])), orig) {
		t.Fatal("the restored backup was consumed")
	}
	if r := listBackups(root)[0]; !strings.HasPrefix(r, "01_replaced_") || !sameHashes(hashDir(filepath.Join(root, backupDir, r)), swirl) {
		t.Fatalf("the replaced SWIRL menu was not kept: %v", listBackups(root))
	}
	// going back to SWIRL from the restored original does not pin a second original
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range listBackups(root) {
		if strings.HasPrefix(b, "01_original_") {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("%d original backups: %v", n, listBackups(root))
	}
}

func TestMenuSwapRecovery(t *testing.T) {
	// both the swap and its undo fail: RECOVER.json names both folders and the next scan puts 01 back
	root := txnCard(t, "openMenu", 2)
	orig := hashDir(filepath.Join(root, "01"))
	_, done := useFaults(t, &fault{op: "Rename", n: 2, sticky: true, err: syscall.EACCES})
	err := InstallSwirl(root, "", quiet)
	done()
	if err == nil || !strings.Contains(err.Error(), "could not be put back") {
		t.Fatalf("install: %v", err)
	}
	b, rerr := os.ReadFile(recoverPath(root))
	if rerr != nil {
		t.Fatal("no RECOVER.json:", rerr)
	}
	var info recoverInfo
	if json.Unmarshal(b, &info) != nil || !strings.HasPrefix(info.Previous, backupDir+"/01_original_openmenu_") || info.New != editsDir+"/"+stageName {
		t.Fatalf("RECOVER.json: %s", b)
	}
	// not while the card is locked
	unlock, _ := lockCard(root, "Update SWIRL")
	ScanCard(root)
	if fileExists(filepath.Join(root, "01")) {
		t.Fatal("a scan repaired the card while it was locked")
	}
	unlock()
	c, err := ScanCard(root)
	if err != nil {
		t.Fatal(err)
	}
	if !sameHashes(hashDir(filepath.Join(root, "01")), orig) || fileExists(recoverPath(root)) || fileExists(stagePath(root)) {
		t.Fatalf("the scan did not put the old menu back: %v", c.Warnings)
	}
	if len(c.Warnings) == 0 || !strings.Contains(c.Warnings[0], "undone") {
		t.Fatalf("warnings %q", c.Warnings)
	}
	// a staging folder left by a crash is removed by the next scan and reported
	os.MkdirAll(stagePath(root), 0o755)
	os.WriteFile(filepath.Join(stagePath(root), "track05.iso"), []byte("half"), 0o644)
	c, _ = ScanCard(root)
	if fileExists(stagePath(root)) || len(c.Warnings) == 0 || !strings.Contains(c.Warnings[0], ".stage_01") {
		t.Fatalf("stale stage: %v", c.Warnings)
	}
}

// folderMap names the game in each folder (folder -> serial), for comparing a card before and after.
func folderMap(root string) string {
	c, err := ScanCard(root)
	if err != nil {
		return err.Error()
	}
	var out []string
	for _, g := range c.Games {
		out = append(out, g.Folder+"="+g.Product)
	}
	return strings.Join(out, " ")
}

// A4: a renumbering cut short at any step (power lost, card pulled) is undone by the next scan.
func TestRenumberJournal(t *testing.T) {
	for n := 1; n <= 12; n++ {
		t.Run(fmt.Sprintf("power_lost_at_rename_%d", n), func(t *testing.T) {
			root := txnCard(t, "swirl", 5)
			os.WriteFile(artPath(root, "05", "box"), pngBytes(quadImage(32)), 0o644)
			s := snapCard(root)
			before := folderMap(root)
			art03, _ := os.ReadFile(artPath(root, "03", "box"))
			dest := uniqueBackupPath(root, "removed_")
			useFaults(t, &fault{op: "Rename", n: n, panic: true})
			func() {
				defer func() { recover() }()
				renumberTxn(root, []string{"03"}, dest, []string{"02", "04", "05", "06"})
			}()
			cardfs = realFS{}
			if n > 1 { // the first rename puts the journal in place; before it nothing has changed
				if !fileExists(journalPath(root)) {
					t.Fatal("no journal after an interrupted renumbering")
				}
				c, _ := ScanCard(root)
				if len(c.Warnings) == 0 || !strings.Contains(c.Warnings[0], "undone") {
					t.Fatalf("warnings %q", c.Warnings)
				}
			}
			report(t, true, assertCardSane(t, root, s))
			if after := folderMap(root); after != before {
				t.Fatalf("folders %s, were %s", after, before)
			}
			if b, _ := os.ReadFile(artPath(root, "03", "box")); string(b) != string(art03) {
				t.Fatal("the box art of 03 did not come back")
			}
			if fileExists(dest) {
				t.Fatal("the empty removed_ folder was left")
			}
		})
	}
	t.Run("completes", func(t *testing.T) {
		root := txnCard(t, "swirl", 4)
		art03, _ := os.ReadFile(artPath(root, "03", "box"))
		os.WriteFile(artPath(root, "05", "box"), []byte("art of game D"), 0o644)
		moved, err := renumberTxn(root, []string{"02"}, uniqueBackupPath(root, "removed_"), []string{"03", "04", "05"})
		if err != nil || moved != 3 {
			t.Fatalf("moved %d, %v", moved, err)
		}
		if fileExists(journalPath(root)) {
			t.Fatal("journal left after a finished renumbering")
		}
		if m := folderMap(root); m != "02=T00003N 03=T00004N 04=T00005N" {
			t.Fatalf("folders %s", m)
		}
		if b, _ := os.ReadFile(artPath(root, "02", "box")); string(b) != string(art03) {
			t.Fatal("box art did not follow its game")
		}
		if b, _ := os.ReadFile(artPath(root, "04", "box")); string(b) != "art of game D" {
			t.Fatal("box art did not follow its game")
		}
		if e := loadEdits(root).Games; len(e) != 3 || e["02"].Product != "T00003N" {
			t.Fatalf("games.json %v", e)
		}
		if l, _ := filepath.Glob(filepath.Join(root, editsDir, "art", "*.rm")); len(l) != 0 {
			t.Fatalf("removed art left: %v", l)
		}
	})
	t.Run("hidden_folder_reported", func(t *testing.T) {
		root := txnCard(t, "swirl", 3)
		os.Rename(filepath.Join(root, "04"), filepath.Join(root, "_swirl_mv_04"))
		c, _ := ScanCard(root)
		if len(c.Warnings) == 0 || !strings.Contains(c.Warnings[0], "_swirl_mv_04") {
			t.Fatalf("warnings %q", c.Warnings)
		}
	})
}

// A5: an add copies into NN.part and renames it when complete; a copy cut short leaves nothing GDEMU sees,
// and the next scan removes the unfinished folder.
func TestAddThroughPart(t *testing.T) {
	src := t.TempDir()
	writeTestGDI(t, filepath.Join(src, "New Game"), "NEW GAME", "T-00077N")
	root := txnCard(t, "swirl", 2)
	p, err := planAdd(root, []string{filepath.Join(src, "New Game")})
	if err != nil {
		t.Fatal(err)
	}
	// power lost during the third file
	useFaults(t, &fault{op: "Write", path: root, n: 3, panic: true})
	func() {
		defer func() { recover() }()
		runCopy(p, root, func(float64) {})
	}()
	cardfs = realFS{}
	if fileExists(filepath.Join(root, "04")) || !fileExists(filepath.Join(root, "04.part")) {
		t.Fatalf("after the cut: %v", listDir(root))
	}
	c, _ := ScanCard(root)
	if len(c.Games) != 2 || fileExists(filepath.Join(root, "04.part")) || len(c.Warnings) == 0 || !strings.Contains(c.Warnings[0], "04.part") {
		t.Fatalf("scan: %d games, warnings %q, %v", len(c.Games), c.Warnings, listDir(root))
	}
	// the same add then works, and leaves no .part behind
	if err := runCopy(p, root, func(float64) {}); err != nil {
		t.Fatal(err)
	}
	if !fileExists(filepath.Join(root, "04", "track03.bin")) || len(partFolders(root)) != 0 {
		t.Fatalf("after the add: %v", listDir(root))
	}
	if b, _ := os.ReadFile(filepath.Join(root, "04", "name.txt")); len(b) == 0 {
		t.Fatal("name.txt was not written into the new folder")
	}
}

// A6: every file written to the card is flushed, and so is its folder, before the step that depends on it.
func TestCardWritesAreFlushed(t *testing.T) {
	count := func(root string) (*fault, *fault) {
		return &fault{op: "SyncFile", path: root, n: 1 << 30}, &fault{op: "SyncDir", path: root, n: 1 << 30}
	}
	root := txnCard(t, "swirl", 3)
	files, dirs := count(root)
	_, done := useFaults(t, files, dirs)
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	done()
	if files.seen != 7 || dirs.seen < 2 { // the 6 menu files and the backup's manifest
		t.Errorf("install flushed %d files and %d folders; want the 6 menu files, the manifest, the stage and the card root", files.seen, dirs.seen)
	}
	src := t.TempDir()
	writeTestGDI(t, filepath.Join(src, "New Game"), "NEW GAME", "T-00077N")
	files, dirs = count(root)
	_, done = useFaults(t, files, dirs)
	if err := StartAddGames(root, []string{filepath.Join(src, "New Game")}, ""); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	done()
	if files.seen < 4+1+6 || dirs.seen < 2 { // 4 game files, name.txt, the rebuilt menu
		t.Errorf("add flushed %d files and %d folders", files.seen, dirs.seen)
	}
	files, dirs = count(root)
	_, done = useFaults(t, files, dirs)
	if err := StartRemoveGames(root, []string{"02"}); err != nil {
		t.Fatal(err)
	}
	if err := waitJobErr(t); err != nil {
		t.Fatal(err)
	}
	done()
	if files.seen < 3 || dirs.seen < 4 { // journal writes, games.json; the folders they are in
		t.Errorf("remove flushed %d files and %d folders", files.seen, dirs.seen)
	}
}

// A6: a flush that fails is a failed write like any other
func TestCardFaultsSync(t *testing.T) {
	var rows []txnRow
	for _, op := range []string{"SyncFile", "SyncDir"} {
		for n := 1; n <= 3; n++ {
			op, n := op, n
			fl := func(root string) []*fault { return []*fault{{op: op, path: root, n: n, err: syscall.ENOSPC}} }
			rows = append(rows,
				txnRow{name: fmt.Sprintf("install_%s_fails_at_%d", op, n), fixed: true, menu: "openMenu", games: 3, faults: fl, run: opInstall, extra: keepsOriginal},
				txnRow{name: fmt.Sprintf("remove_%s_fails_at_%d", op, n), fixed: true, menu: "swirl", games: 4, faults: fl, run: opRemove("03")},
			)
		}
	}
	runTxnRows(t, rows)
}
