package main

// Online art and info: the community openMenu databases publish ready made BOX.DAT, ICON.DAT and META.DAT
// files keyed by disc serial. SWIRL downloads them once to this PC and fills in only what a card is missing.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var dbSources = []struct{ Name, URL string }{
	{"BOX.DAT", "https://github.com/mrneo240/openMenu_imagedb/releases/latest/download/BOX.DAT"},
	{"ICON.DAT", "https://github.com/mrneo240/openMenu_imagedb/releases/latest/download/ICON.DAT"},
	{"META.DAT", "https://github.com/mrneo240/openMenu_metadb/releases/latest/download/META.DAT"},
}

type DBStatus struct {
	Present    bool   `json:"present"`
	Downloaded string `json:"downloaded,omitempty"`
	Art        int    `json:"art"`
	Info       int    `json:"info"`
	// for the card that was asked about
	CanFillArt  int `json:"canFillArt"`
	CanFillInfo int `json:"canFillInfo"`
}

var dbDirOverride string // tests

func dbDir() string {
	if dbDirOverride != "" {
		return dbDirOverride
	}
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "SWIRL Card Manager", "db")
}

type dbInfo struct {
	Downloaded time.Time `json:"downloaded"`
	Art, Info  int
}

func readDBInfo() (*dbInfo, bool) {
	b, err := os.ReadFile(filepath.Join(dbDir(), "info.json"))
	if err != nil {
		return nil, false
	}
	var i dbInfo
	if json.Unmarshal(b, &i) != nil {
		return nil, false
	}
	for _, s := range dbSources {
		if !fileExists(filepath.Join(dbDir(), s.Name)) {
			return nil, false
		}
	}
	return &i, true
}

// datIndex reads just the ID table of a DAT file.
func datIndex(path string) map[string]bool {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(f, hdr); err != nil || string(hdr[:3]) != "DAT" {
		return nil
	}
	n := int(uint32(hdr[8]) | uint32(hdr[9])<<8 | uint32(hdr[10])<<16 | uint32(hdr[11])<<24)
	if n > 100000 {
		return nil
	}
	tab := make([]byte, 16*n)
	if _, err := io.ReadFull(f, tab); err != nil {
		return nil
	}
	out := map[string]bool{}
	for i := 0; i < n; i++ {
		id := tab[16*i : 16*i+12]
		end := 0
		for end < 12 && id[end] != 0 {
			end++
		}
		out[string(id[:end])] = true
	}
	return out
}

func GetDBStatus(root string) DBStatus {
	var s DBStatus
	info, ok := readDBInfo()
	if !ok {
		return s
	}
	s.Present, s.Downloaded, s.Art, s.Info = true, info.Downloaded.Format("Jan 2, 2006"), info.Art, info.Info
	if root == "" {
		return s
	}
	c, err := ScanCard(root)
	if err != nil {
		return s
	}
	box := datIndex(filepath.Join(dbDir(), "BOX.DAT"))
	meta := datIndex(filepath.Join(dbDir(), "META.DAT"))
	for _, g := range c.Games {
		if !g.HasArt && box[g.Product] {
			s.CanFillArt++
		}
		if !g.HasMeta && meta[g.Product] {
			s.CanFillInfo++
		}
	}
	return s
}

// DownloadDB fetches the three database files into the cache folder. progress gets 0..1.
func DownloadDB(log Logger, progress func(float64)) error {
	dir := dbDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 15 * time.Minute}
	// approximate sizes so the bar moves sensibly before each file reports its length
	weights := []float64{0.78, 0.2, 0.02}
	done := 0.0
	for i, s := range dbSources {
		log("Downloading %s", s.Name)
		resp, err := client.Get(s.URL)
		if err != nil {
			return fmt.Errorf("could not reach GitHub (%v); check the PC's internet connection", err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return fmt.Errorf("downloading %s failed: %s", s.Name, resp.Status)
		}
		tmp := filepath.Join(dir, s.Name+".part")
		out, err := os.Create(tmp)
		if err != nil {
			resp.Body.Close()
			return err
		}
		var got int64
		buf := make([]byte, 1<<20)
		for {
			n, rerr := resp.Body.Read(buf)
			if n > 0 {
				if _, err := out.Write(buf[:n]); err != nil {
					out.Close()
					resp.Body.Close()
					return err
				}
				got += int64(n)
				if resp.ContentLength > 0 {
					progress(done + weights[i]*float64(got)/float64(resp.ContentLength))
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				out.Close()
				resp.Body.Close()
				return fmt.Errorf("download of %s was interrupted: %v", s.Name, rerr)
			}
		}
		out.Close()
		resp.Body.Close()
		if _, err := readDatHeader(tmp); err != nil {
			os.Remove(tmp)
			return fmt.Errorf("%s from GitHub is not a valid DAT file", s.Name)
		}
		os.Remove(filepath.Join(dir, s.Name))
		if err := os.Rename(tmp, filepath.Join(dir, s.Name)); err != nil {
			return err
		}
		done += weights[i]
		progress(done)
	}
	info := dbInfo{Downloaded: time.Now(), Art: len(datIndex(filepath.Join(dir, "BOX.DAT"))), Info: len(datIndex(filepath.Join(dir, "META.DAT")))}
	b, _ := json.Marshal(info)
	os.WriteFile(filepath.Join(dir, "info.json"), b, 0o644)
	log("Database has box art for %d games and info for %d", info.Art, info.Info)
	return nil
}

func readDatHeader(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(f, hdr); err != nil || string(hdr[:3]) != "DAT" || hdr[3] != 1 {
		return 0, errors.New("not a DAT file")
	}
	return int(uint32(hdr[4]) | uint32(hdr[5])<<8 | uint32(hdr[6])<<16 | uint32(hdr[7])<<24), nil
}

// mergeOnlineDB adds database art and info for games whose entries are missing from the menu data.
// Nothing already on the card is replaced, and SWIRL edits are applied afterwards so they always win.
func mergeOnlineDB(data string, c *Card, log Logger) error {
	if _, ok := readDBInfo(); !ok {
		return nil
	}
	type dat struct {
		name  string
		chunk int
	}
	counts := map[string]int{}
	for _, d := range []dat{{"BOX.DAT", 131104}, {"ICON.DAT", 32800}, {"META.DAT", metaSize}} {
		idx := datIndex(filepath.Join(dbDir(), d.name))
		card, path := loadOrNewDat(data, d.name, d.chunk)
		var want []string
		for _, g := range c.Games {
			if g.Product == "" || !idx[g.Product] {
				continue
			}
			if _, have := card.Chunks[g.Product]; have {
				continue
			}
			want = append(want, g.Product)
		}
		if len(want) == 0 {
			continue
		}
		chunks, err := readDatChunks(filepath.Join(dbDir(), d.name), d.chunk, want)
		if err != nil {
			return err
		}
		for _, id := range want {
			if b := chunks[id]; b != nil {
				card.Set(id, b)
			}
		}
		if err := card.Write(path); err != nil {
			return err
		}
		counts[d.name] = len(want)
	}
	if counts["BOX.DAT"] > 0 || counts["META.DAT"] > 0 {
		log("Added online box art for %d games and info for %d", counts["BOX.DAT"], counts["META.DAT"])
	}
	return nil
}

// readDatChunks reads only the requested chunks, so the 90 MB box art file is never loaded whole.
func readDatChunks(path string, chunk int, ids []string) (map[string][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	hdr := make([]byte, 16)
	if _, err := io.ReadFull(f, hdr); err != nil || string(hdr[:3]) != "DAT" {
		return nil, errors.New("bad database file " + path)
	}
	cs := int(binary.LittleEndian.Uint32(hdr[4:]))
	n := int(binary.LittleEndian.Uint32(hdr[8:]))
	if cs != chunk || n > 100000 {
		return nil, errors.New("unexpected layout in " + path)
	}
	tab := make([]byte, 16*n)
	if _, err := io.ReadFull(f, tab); err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	out := map[string][]byte{}
	for i := 0; i < n; i++ {
		rec := tab[16*i : 16*i+16]
		id := string(bytes.TrimRight(rec[:12], "\x00"))
		if !want[id] || out[id] != nil {
			continue
		}
		b := make([]byte, cs)
		if _, err := f.ReadAt(b, int64(binary.LittleEndian.Uint32(rec[12:]))*int64(cs)); err != nil {
			return nil, err
		}
		out[id] = b
	}
	return out, nil
}
