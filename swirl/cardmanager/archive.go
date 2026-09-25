package main

// Compressed games: .zip, .7z and .rar archives. Games inside them are found from the archive's file list
// and unpacked straight into the new game folder on the SD card, so nothing needs to be unpacked on the PC.

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/bodgit/sevenzip"
	"github.com/nwaples/rardecode/v2"
)

var archiveExts = map[string]bool{".zip": true, ".7z": true, ".rar": true}

// multi part RAR sets: only the first part is shown and opened
var rarPartRe = regexp.MustCompile(`(?i)\.part0*(\d+)\.rar$`)
var rarOldPartRe = regexp.MustCompile(`(?i)\.r\d\d$`)

func isArchive(name string) bool {
	if !archiveExts[strings.ToLower(filepath.Ext(name))] {
		return false
	}
	if m := rarPartRe.FindStringSubmatch(name); m != nil && m[1] != "1" {
		return false
	}
	return true
}

func isArchivePart(name string) bool {
	if rarOldPartRe.MatchString(name) {
		return true
	}
	m := rarPartRe.FindStringSubmatch(name)
	return m != nil && m[1] != "1"
}

type arcEntry struct {
	Name string // forward slashes, as stored
	Size int64
}

func junkEntry(name string) bool {
	base := path.Base(name)
	return strings.HasPrefix(name, "__MACOSX/") || strings.HasPrefix(base, "._") || base == ".DS_Store" || base == "Thumbs.db" || base == "desktop.ini"
}

// listArchive reads the archive's table of contents (no unpacking).
func listArchive(p string) ([]arcEntry, error) {
	var out []arcEntry
	switch strings.ToLower(filepath.Ext(p)) {
	case ".zip":
		r, err := zip.OpenReader(p)
		if err != nil {
			return nil, archiveErr(p, err)
		}
		defer r.Close()
		for _, f := range r.File {
			if !f.FileInfo().IsDir() {
				out = append(out, arcEntry{cleanEntry(f.Name), int64(f.UncompressedSize64)})
			}
		}
	case ".7z":
		r, err := sevenzip.OpenReader(p)
		if err != nil {
			return nil, archiveErr(p, err)
		}
		defer r.Close()
		for _, f := range r.File {
			if !f.FileInfo().IsDir() {
				out = append(out, arcEntry{cleanEntry(f.Name), int64(f.UncompressedSize)})
			}
		}
	case ".rar":
		files, err := rardecode.List(p)
		if err != nil {
			return nil, archiveErr(p, err)
		}
		for _, f := range files {
			if f.Encrypted || f.HeaderEncrypted {
				return nil, fmt.Errorf("%s is password protected", filepath.Base(p))
			}
			if !f.IsDir {
				out = append(out, arcEntry{cleanEntry(f.Name), f.UnPackedSize})
			}
		}
	default:
		return nil, fmt.Errorf("%s is not a .zip, .7z or .rar file", filepath.Base(p))
	}
	return out, nil
}

func archiveErr(p string, err error) error {
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "password") || strings.Contains(strings.ToLower(msg), "encrypt") {
		return fmt.Errorf("%s is password protected", filepath.Base(p))
	}
	return fmt.Errorf("%s could not be opened (%v)", filepath.Base(p), err)
}

func cleanEntry(n string) string {
	n = strings.ReplaceAll(n, `\`, "/")
	return strings.TrimPrefix(path.Clean("/"+n), "/")
}

var errStopWalk = errors.New("stop")

// walkArchive streams the archive's files in stored order, which is the fast order for solid 7z and RAR
// archives. fn is called for each wanted file; returning errStopWalk ends the walk early.
func walkArchive(p string, want func(name string) bool, fn func(name string, size int64, r io.Reader) error) error {
	done := func(err error) error {
		if err == errStopWalk {
			return nil
		}
		return err
	}
	switch strings.ToLower(filepath.Ext(p)) {
	case ".zip":
		r, err := zip.OpenReader(p)
		if err != nil {
			return archiveErr(p, err)
		}
		defer r.Close()
		for _, f := range r.File {
			name := cleanEntry(f.Name)
			if f.FileInfo().IsDir() || !want(name) {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return archiveErr(p, err)
			}
			err = fn(name, int64(f.UncompressedSize64), rc)
			rc.Close()
			if err != nil {
				return done(err)
			}
		}
	case ".7z":
		r, err := sevenzip.OpenReader(p)
		if err != nil {
			return archiveErr(p, err)
		}
		defer r.Close()
		for _, f := range r.File {
			name := cleanEntry(f.Name)
			if f.FileInfo().IsDir() || !want(name) {
				continue
			}
			rc, err := f.Open()
			if err != nil {
				return archiveErr(p, err)
			}
			err = fn(name, int64(f.UncompressedSize), rc)
			rc.Close()
			if err != nil {
				return done(err)
			}
		}
	case ".rar":
		r, err := rardecode.OpenReader(p)
		if err != nil {
			return archiveErr(p, err)
		}
		defer r.Close()
		for {
			h, err := r.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return archiveErr(p, err)
			}
			name := cleanEntry(h.Name)
			if h.IsDir || !want(name) {
				continue
			}
			if err := fn(name, h.UnPackedSize, r); err != nil {
				return done(err)
			}
		}
	default:
		return fmt.Errorf("%s is not a .zip, .7z or .rar file", filepath.Base(p))
	}
	return nil
}

// readArchiveFile returns the first max bytes of one file in the archive.
func readArchiveFile(p, name string, max int) ([]byte, error) {
	var out []byte
	found := false
	err := walkArchive(p, func(n string) bool { return n == name }, func(_ string, _ int64, r io.Reader) error {
		found = true
		b, err := io.ReadAll(io.LimitReader(r, int64(max)))
		out = b
		if err != nil {
			return err
		}
		return errStopWalk
	})
	if err == nil && !found {
		err = fmt.Errorf("%s is not in %s", name, filepath.Base(p))
	}
	return out, err
}

// arcGame is one game found inside an archive.
type arcGame struct {
	Name   string     `json:"name"`   // label for the game
	Image  string     `json:"image"`  // the .gdi / .cdi / .mds / .ccd entry
	Format string     `json:"format"` // GDI, CDI, ...
	Files  []arcEntry `json:"-"`
	Bytes  int64      `json:"bytes"`
}

// gamesInArchive works out which files belong to which game. A game is a disc image plus the other files
// in its folder inside the archive. Folders with more than one CDI keep each CDI as its own game.
func gamesInArchive(p string, entries []arcEntry) []arcGame {
	byDir := map[string][]arcEntry{}
	images := map[string][]arcEntry{}
	var dirs []string
	for _, e := range entries {
		if junkEntry(e.Name) {
			continue
		}
		d := path.Dir(e.Name)
		if _, ok := byDir[d]; !ok {
			dirs = append(dirs, d)
		}
		byDir[d] = append(byDir[d], e)
		if imageExts[strings.ToLower(path.Ext(e.Name))] {
			images[d] = append(images[d], e)
		}
	}
	sort.Strings(dirs)
	fname := filepath.Base(p)
	base := strings.TrimSuffix(fname, filepath.Ext(fname))
	if m := rarPartRe.FindStringIndex(fname); m != nil {
		base = fname[:m[0]]
	}
	var out []arcGame
	for _, d := range dirs {
		imgs := images[d]
		if len(imgs) == 0 {
			continue
		}
		// prefer the .gdi when a folder has several image types (a .gdi next to a .cue, say)
		split := len(imgs) > 1
		for _, im := range imgs {
			g := arcGame{Image: im.Name, Format: strings.ToUpper(strings.TrimPrefix(path.Ext(im.Name), "."))}
			if split {
				stem := strings.TrimSuffix(im.Name, path.Ext(im.Name))
				for _, f := range byDir[d] {
					if f.Name == im.Name || strings.HasPrefix(f.Name, stem+".") {
						g.Files = append(g.Files, f)
					}
				}
				g.Name = strings.TrimSuffix(path.Base(im.Name), path.Ext(im.Name))
			} else {
				g.Files = append(g.Files, byDir[d]...)
				g.Name = base
				// the nearest folder with a real name, so "Game/GDI/disc.gdi" is called Game
				for x := d; x != "." && x != "/"; x = path.Dir(x) {
					if !genericName(path.Base(x)) {
						if len(images) > 1 || len(dirs) > 0 {
							g.Name = path.Base(x)
						}
						break
					}
				}
			}
			if genericName(g.Name) {
				g.Name = base
			}
			for _, f := range g.Files {
				g.Bytes += f.Size
			}
			out = append(out, g)
		}
	}
	return out
}

// names like "disc" or "GDI" say nothing about the game
func genericName(n string) bool {
	switch strings.ToLower(n) {
	case "disc", "disk", "gdi", "cdi", "game", "track", ".", "":
		return true
	}
	return false
}

// archiveIP reads the IP.BIN of a GDI game inside a .zip without unpacking it (the other formats are
// checked after unpacking, because reaching the data track would mean unpacking most of the archive).
func archiveIP(p string, g arcGame) (*ipInfo, bool) {
	if strings.ToLower(filepath.Ext(p)) != ".zip" || g.Format != "GDI" {
		return nil, false
	}
	text, err := readArchiveFile(p, g.Image, 64<<10)
	if err != nil {
		return nil, false
	}
	dir := path.Dir(g.Image)
	for i, line := range strings.Split(string(text), "\n") {
		f := splitGDILine(strings.TrimSpace(line))
		if i == 0 || len(f) < 5 || f[1] != "45000" {
			continue
		}
		b, err := readArchiveFile(p, cleanEntry(path.Join(dir, f[4])), 2352)
		if err != nil || len(b) < 0x110 {
			return nil, false
		}
		off := 0
		if f[3] == "2352" {
			off = 16
		}
		ip, ok := parseIP(b[off:])
		return ip, ok
	}
	return nil, false
}

// ---------- peek: what the file browser shows for an archive or a game folder ----------

type PeekResult struct {
	Games   []arcGame `json:"games"`
	Name    string    `json:"name,omitempty"`    // disc title when it could be read
	Product string    `json:"product,omitempty"` // serial
	OnCard  string    `json:"onCard,omitempty"`  // folder on the card that already has this game
	Bytes   int64     `json:"bytes"`
	Error   string    `json:"error,omitempty"`
}

type peekKey struct {
	path, root string
	mod        int64
}

var peekCache sync.Map

func Peek(p, root string) PeekResult {
	st, err := os.Stat(p)
	if err != nil {
		return PeekResult{Error: "not found"}
	}
	k := peekKey{p, root, st.ModTime().UnixNano()}
	if v, ok := peekCache.Load(k); ok {
		return v.(PeekResult)
	}
	r := peek(p, st, root)
	peekCache.Store(k, r)
	return r
}

func peek(p string, st os.FileInfo, root string) PeekResult {
	var r PeekResult
	var ip *ipInfo
	if st.IsDir() || imageExts[strings.ToLower(filepath.Ext(p))] {
		dir := p
		if !st.IsDir() {
			dir = filepath.Dir(p)
		}
		ip, _, _ = readImageIP(dir)
		r.Bytes, _ = folderSize(dir)
	} else {
		entries, err := listArchive(p)
		if err != nil {
			r.Error = err.Error()
			return r
		}
		r.Games = gamesInArchive(p, entries)
		for _, g := range r.Games {
			r.Bytes += g.Bytes
		}
		if len(r.Games) == 1 {
			ip, _ = archiveIP(p, r.Games[0])
		}
	}
	if ip != nil {
		r.Name, r.Product = ip.Name, ip.Product
		if root != "" {
			r.OnCard = cardHasGame(root, ip)
		}
	}
	return r
}

// ---------- games already on the card ----------

func ipKey(ip *ipInfo) string {
	if ip == nil || ip.Product == "" {
		return ""
	}
	// serial and disc number: another revision of the same disc counts as the same game
	return strings.ToUpper(strings.ReplaceAll(ip.Product, "-", "")) + "|" + ip.Disc
}

type cardKeys struct {
	mu   sync.Mutex
	root string
	at   int64
	keys map[string]string // key -> folder
}

var cardKeyCache cardKeys

// cardGameKeys lists the games on the card by serial and disc number. Cached for a short while because
// the file browser asks for every game it shows.
func cardGameKeys(root string) map[string]string {
	c := &cardKeyCache
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now().Unix()
	if c.root == root && c.keys != nil && now-c.at < 20 {
		return c.keys
	}
	keys := map[string]string{}
	for _, n := range numberedFolders(root) {
		f := folderName(root, n)
		if ip, _, err := readImageIP(filepath.Join(root, f)); err == nil {
			if k := ipKey(ip); k != "" {
				if _, dup := keys[k]; !dup {
					keys[k] = f
				}
			}
		}
	}
	c.root, c.at, c.keys = root, now, keys
	return keys
}

func forgetCardKeys() {
	cardKeyCache.mu.Lock()
	cardKeyCache.keys = nil
	cardKeyCache.mu.Unlock()
}

func cardHasGame(root string, ip *ipInfo) string {
	k := ipKey(ip)
	if k == "" || root == "" {
		return ""
	}
	return cardGameKeys(root)[k]
}
