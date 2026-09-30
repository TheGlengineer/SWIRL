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
	Dir  string `json:"-"` // "" for a built in patch, else the folder holding File
}

// userPatchDir is a folder where the owner can put their own patches: a catalog.json in the same shape as the
// built in one, next to the .dcp files it names. Tests point SWIRL_PATCH_DIR at a folder of their own.
func userPatchDir() string {
	if d := os.Getenv("SWIRL_PATCH_DIR"); d != "" {
		return d
	}
	return filepath.Join(appDataDir(), "patches")
}

func vgaCatalog() []vgaCatalogEntry {
	var out []vgaCatalogEntry
	if b, err := patchFiles.ReadFile("patches/catalog.json"); err == nil {
		json.Unmarshal(b, &out)
	}
	if b, err := os.ReadFile(filepath.Join(userPatchDir(), "catalog.json")); err == nil {
		var extra []vgaCatalogEntry
		if json.Unmarshal(b, &extra) == nil {
			for i := range extra {
				extra[i].Dir = userPatchDir()
			}
			out = append(extra, out...) // the owner's entries win
		}
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
	for i := range plain {
		if &plain[i] == e {
			return &cat[i]
		}
	}
	return nil
}

// writeCatalogPatch puts the embedded .dcp in a temporary file and returns its path.
func writeCatalogPatch(e *vgaCatalogEntry) (string, error) {
	var b []byte
	var err error
	if e.Dir != "" {
		b, err = os.ReadFile(filepath.Join(e.Dir, e.File))
	} else {
		b, err = patchFiles.ReadFile("patches/" + e.File)
	}
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

// vgaFixAll applies the catalog patch to every game on the card that has one and is not patched yet.
// One line per game says what happened; a game that fails does not stop the others.
func vgaFixAll(root string, dry bool) ([]string, error) {
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
			continue
		}
		e := vgaPatchFor(ip.Product, ip.Version)
		if e == nil {
			continue
		}
		name := fmt.Sprintf("%s %s (%s %s)", en.Name(), strings.TrimSpace(ip.Name), ip.Product, ip.Version)
		rep, err := applyCatalogVGAPatch(root, folder, dry)
		switch {
		case err != nil:
			rows = append(rows, fmt.Sprintf("%s: not patched: %v", name, err))
		case rep.Sectors == 0:
			rows = append(rows, fmt.Sprintf("%s: already patched (%s)", name, e.Author))
		case dry:
			rows = append(rows, fmt.Sprintf("%s: would patch %d sectors (%s)", name, rep.Sectors, e.Author))
		default:
			rows = append(rows, fmt.Sprintf("%s: patched, %d sectors (%s), undo in %s", name, rep.Sectors, e.Author, rep.UndoFile))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, "no game on this card has a known VGA patch")
	}
	return rows, nil
}

type vgaFixResult struct {
	Patched []string `json:"patched"` // game names patched now
	Already []string `json:"already"`
	Failed  []string `json:"failed"` // "name: why"
	Log     []string `json:"log"`
}

// vgaFixCard patches every game with a known patch that is not patched yet, for the page.
func vgaFixCard(root string) (*vgaFixResult, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	res := &vgaFixResult{Patched: []string{}, Already: []string{}, Failed: []string{}, Log: []string{}}
	for _, en := range entries {
		if !en.IsDir() || !folderRe.MatchString(en.Name()) || en.Name() == "01" {
			continue
		}
		folder := filepath.Join(root, en.Name())
		ip, _, err := readImageIP(folder)
		if err != nil {
			continue
		}
		e := vgaPatchFor(ip.Product, ip.Version)
		if e == nil {
			continue
		}
		name := strings.TrimSpace(ip.Name)
		if n := readText(filepath.Join(folder, "name.txt")); n != "" {
			name = n
		}
		if vgaUndoFile(root, folder) != "" {
			res.Already = append(res.Already, name)
			continue
		}
		if vgaSkipped(root, folder) {
			res.Log = append(res.Log, fmt.Sprintf("%s (folder %s): left alone, its patch was removed by hand", name, en.Name()))
			continue
		}
		rep, err := applyCatalogVGAPatch(root, folder, false)
		if err != nil {
			res.Failed = append(res.Failed, name+": "+err.Error())
			res.Log = append(res.Log, fmt.Sprintf("%s (folder %s): not patched: %v", name, en.Name(), err))
			continue
		}
		res.Patched = append(res.Patched, name)
		res.Log = append(res.Log, fmt.Sprintf("%s (folder %s): VGA patch by %s applied, %d sectors, undo in %s", name, en.Name(), e.Author, rep.Sectors, filepath.Base(rep.UndoFile)))
	}
	return res, nil
}
