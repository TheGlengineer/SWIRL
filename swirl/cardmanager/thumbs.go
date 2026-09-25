package main

// Cover thumbnails for the Games grid. Art comes from the owner's own edit, else the menu disc's
// ICON.DAT (128x128) or BOX.DAT. The menu disc is opened once per card and kept open, and finished PNGs
// are kept in memory, so a grid of hundreds of games loads quickly.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"sync"
)

type thumbCard struct {
	stamp string // menu disc file time; a rebuilt menu starts afresh
	menu  *menuReader
	pngs  map[string][]byte
}

var (
	thumbMu    sync.Mutex
	thumbCards = map[string]*thumbCard{}
)

func menuStamp(root string) string {
	gdi := findGDI(filepath.Join(root, "01"))
	if gdi == "" {
		return ""
	}
	st, err := os.Stat(gdi)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d", st.ModTime().UnixNano(), st.Size())
}

// GameThumb returns a PNG of the game's cover for the grid: 128 pixels square, or 256 when big.
func GameThumb(root, folder, product string, big bool) ([]byte, error) {
	size := 128
	if big {
		size = 256
	}
	if !folderRe.MatchString(folder) {
		return nil, os.ErrNotExist
	}
	thumbMu.Lock()
	defer thumbMu.Unlock()
	stamp := menuStamp(root)
	tc := thumbCards[root]
	if tc == nil || tc.stamp != stamp {
		if tc != nil && tc.menu != nil {
			tc.menu.Close()
		}
		tc = &thumbCard{stamp: stamp, pngs: map[string][]byte{}}
		if stamp != "" {
			tc.menu, _ = openMenuDisc(root)
		}
		thumbCards[root] = tc
	}
	// the owner's own box art, as edited in Card Manager
	if st, err := os.Stat(artPath(root, folder, "box")); err == nil {
		key := fmt.Sprintf("edit|%s|%d|%d", folder, st.ModTime().UnixNano(), size)
		if b, ok := tc.pngs[key]; ok {
			return b, nil
		}
		raw, err := os.ReadFile(artPath(root, folder, "box"))
		if err != nil {
			return nil, err
		}
		img, _, err := image.Decode(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		b := pngBytes(fitSquare(img, size))
		tc.pngs[key] = b
		return b, nil
	}
	if product == "" || tc.menu == nil {
		return nil, os.ErrNotExist
	}
	key := fmt.Sprintf("dat|%s|%d", product, size)
	if b, ok := tc.pngs[key]; ok {
		if b == nil {
			return nil, os.ErrNotExist
		}
		return b, nil
	}
	first, second := "ICON.DAT", "BOX.DAT"
	if big {
		first, second = second, first
	}
	raw, err := tc.menu.datChunk(first, product)
	if err != nil {
		raw, err = tc.menu.datChunk(second, product)
	}
	if err != nil {
		tc.pngs[key] = nil
		return nil, os.ErrNotExist
	}
	img, err := decodePVR(raw)
	if err != nil {
		tc.pngs[key] = nil
		return nil, err
	}
	if img.Bounds().Dx() > size {
		img = fitSquare(img, size)
	}
	b := pngBytes(img)
	tc.pngs[key] = b
	return b, nil
}

// ---------- small settings for the app's own screens ----------

type UIPrefs struct {
	GamesView string `json:"gamesView"` // list or grid
	GridSize  int    `json:"gridSize"`  // tile width in pixels
	BackupDir string `json:"backupDir"` // where the last card backup went
	BackupOld bool   `json:"backupOld"` // it included SWIRL_BACKUP
	// AutoUpdate turns the daily check for a newer version on GitHub on or off (on when unset)
	AutoUpdate *bool  `json:"autoUpdate,omitempty"`
	SkipUpdate string `json:"skipUpdate,omitempty"` // a version the owner chose not to be told about again
}

func (p UIPrefs) UpdateAuto() bool { return p.AutoUpdate == nil || *p.AutoUpdate }

var uiPrefsMu sync.Mutex

func uiPrefsPath() string { return filepath.Join(appDataDir(), "ui.json") }

func LoadUIPrefs() UIPrefs {
	uiPrefsMu.Lock()
	defer uiPrefsMu.Unlock()
	p := UIPrefs{GamesView: "list", GridSize: 150}
	if b, err := os.ReadFile(uiPrefsPath()); err == nil {
		json.Unmarshal(b, &p)
	}
	if p.GamesView != "grid" {
		p.GamesView = "list"
	}
	if p.GridSize < 100 || p.GridSize > 260 {
		p.GridSize = 150
	}
	return p
}

func SaveUIPrefs(p UIPrefs) {
	uiPrefsMu.Lock()
	defer uiPrefsMu.Unlock()
	os.MkdirAll(appDataDir(), 0o755)
	b, _ := json.MarshalIndent(p, "", "  ")
	os.WriteFile(uiPrefsPath(), b, 0o644)
}
