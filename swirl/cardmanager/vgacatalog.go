package main

// The VGA patch catalog: community patches (from the ConsoleMods VGA patch list) that give VGA output to games
// that have none. The .dcp files ship inside Card Manager so applying one needs no download.

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

//go:embed patches/catalog.json patches/*.dcp
var patchFiles embed.FS

type vgaCatalogEntry struct {
	vgaPatchEntry
	File string `json:"file"`
}

func vgaCatalog() []vgaCatalogEntry {
	b, err := patchFiles.ReadFile("patches/catalog.json")
	if err != nil {
		return nil
	}
	var out []vgaCatalogEntry
	if json.Unmarshal(b, &out) != nil {
		return nil
	}
	return out
}

// vgaPatchFor finds the catalog patch for a game by its IP.BIN product and version.
func vgaPatchFor(product, version string) *vgaCatalogEntry {
	cat := vgaCatalog()
	plain := make([]vgaPatchEntry, len(cat))
	for i := range cat {
		plain[i] = cat[i].vgaPatchEntry
	}
	e := findVGAPatch(plain, product, version)
	if e == nil {
		return nil
	}
	for i := range cat {
		if cat[i].Product == e.Product && cat[i].Version == e.Version {
			return &cat[i]
		}
	}
	return nil
}

// writeCatalogPatch puts the embedded .dcp in a temporary file and returns its path.
func writeCatalogPatch(e *vgaCatalogEntry) (string, error) {
	b, err := patchFiles.ReadFile("patches/" + e.File)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "swirl_patch_")
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, e.File)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		return "", err
	}
	if err := checkDCPHash(p, e.SHA256); err != nil {
		return "", err
	}
	return p, nil
}

// applyCatalogVGAPatch patches a game folder with the catalog's patch for its product and version.
func applyCatalogVGAPatch(root, folder string, dry bool) (*patchReport, error) {
	ip, _, err := readImageIP(folder)
	if err != nil {
		return nil, err
	}
	e := vgaPatchFor(ip.Product, ip.Version)
	if e == nil {
		return nil, fmt.Errorf("no VGA patch is known for %s (%s %s)", strings.TrimSpace(ip.Name), ip.Product, ip.Version)
	}
	p, err := writeCatalogPatch(e)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(filepath.Dir(p))
	return applyDCP(root, folder, p, dry)
}

// vgaStatusReport lists each game folder's VGA state, one line per folder.
func vgaStatusReport(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var rows []string
	for _, en := range entries {
		if !en.IsDir() || !folderRe.MatchString(en.Name()) || en.Name() == "01" {
			continue
		}
		folder := filepath.Join(root, en.Name())
		ip, _, err := readImageIP(folder)
		if err != nil {
			rows = append(rows, fmt.Sprintf("%s: %v", en.Name(), err))
			continue
		}
		state := "no VGA"
		if ip.VGA {
			state = "VGA"
		}
		if e := vgaPatchFor(ip.Product, ip.Version); e != nil {
			state += ", patch available (" + e.Author + ")"
		}
		if notes, err := os.ReadFile(filepath.Join(folder, "patches.txt")); err == nil && len(notes) > 0 {
			state += ", patched: " + strings.TrimSpace(strings.ReplaceAll(string(notes), "\r\n", "; "))
		}
		rows = append(rows, fmt.Sprintf("%s: %s (%s %s): %s", en.Name(), strings.TrimSpace(ip.Name), ip.Product, ip.Version, state))
	}
	return rows, nil
}
