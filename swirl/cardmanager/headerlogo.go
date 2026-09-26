package main

// openMenu's own files on the menu disc.
//
// SWIRL draws its header logo from openMenu's USA theme picture (THEME/NTSC_U/BG_U_L.PVR), and the Classic
// styles (Classic list, Classic grid, GDMENU) need openMenu's theme pictures, fonts and EMPTY.PVR. Cards
// that had openMenu already have them on the menu disc. Cards set up with GDMENU, from scratch or with
// another tool do not, and a Classic style on such a card stops the Dreamcast.
//
// So Card Manager fetches openMenu's official release from GitHub once, checks its SHA-256, keeps the files
// with the app's data, and adds whichever of them a menu disc is missing. Files already on the disc are never
// replaced, and openMenu's program (1ST_READ.BIN) is never used. Nothing from openMenu is stored in this
// repository. Without a connection, SWIRL still works; it shows its name instead of the logo and the
// Classic styles stay unavailable until the next Update SWIRL with a connection.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var (
	headerLogoZipURL = "https://github.com/mrneo240/openMenu/releases/download/2021-12-4_alpha2/openmenu_12-5-2021_alpha2.zip"
	headerLogoZipSHA = "1a606c2118a1a76dbf86de849e210ee8d3fe54d7dc0fbf242c0cdaae9c5ecac8"
	headerLogoInZip  = "theme/ntsc_u/bg_u_l.pvr"
)

const headerLogoOnDisc = "THEME/NTSC_U/BG_U_L.PVR"

// classicFiles are the openMenu files the Classic styles cannot start without.
var classicFiles = []string{
	"EMPTY.PVR",
	"THEME/SHARED/HIGHLIGHT.PVR", "THEME/SHARED/ICON_WHITE.PVR", "THEME/SHARED/ICON_BLACK.PVR",
	"THEME/GDMENU/BG_L.PVR", "THEME/GDMENU/BG_R.PVR",
	"FONT/BASILEA.FNT", "FONT/BASILEA_W.PVR", "FONT/GDMNUFNT.PVR",
	"THEME/NTSC_U/BG_U_L.PVR", "THEME/NTSC_U/BG_U_R.PVR",
}

func headerLogoCache() string { return filepath.Join(appDataDir(), "openmenu", "BG_U_L.PVR") }

// openMenuFilesDir holds every file from openMenu's release except its program, with upper case names.
func openMenuFilesDir() string { return filepath.Join(appDataDir(), "openmenu", "files") }

const openMenuFilesDone = ".complete"

// findCI finds a path inside dir ignoring case, such as THEME/NTSC_U/BG_U_L.PVR.
func findCI(dir, rel string) string {
	cur := dir
	for _, part := range strings.Split(rel, "/") {
		entries, err := os.ReadDir(cur)
		if err != nil {
			return ""
		}
		found := ""
		for _, e := range entries {
			if strings.EqualFold(e.Name(), part) {
				found = e.Name()
				break
			}
		}
		if found == "" {
			return ""
		}
		cur = filepath.Join(cur, found)
	}
	return cur
}

// fetchOpenMenuFiles downloads openMenu's release once, checks it and unpacks its files (not its program)
// into openMenuFilesDir. Later calls use what is there.
func fetchOpenMenuFiles() (string, error) {
	dir := openMenuFilesDir()
	if b, err := os.ReadFile(filepath.Join(dir, openMenuFilesDone)); err == nil && strings.TrimSpace(string(b)) == headerLogoZipSHA {
		return dir, nil
	}
	if headerLogoZipURL == "" {
		return "", errors.New("downloads are off")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	req, _ := http.NewRequest("GET", headerLogoZipURL, nil)
	req.Header.Set("User-Agent", "SWIRL-Card-Manager/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return "", errors.New("openMenu's release could not be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("openMenu's release answered %s", resp.Status)
	}
	zb, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(zb)
	if hex.EncodeToString(sum[:]) != headerLogoZipSHA {
		return "", errors.New("openMenu's release did not match the expected file")
	}
	zr, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		return "", err
	}
	tmp := dir + ".new"
	os.RemoveAll(tmp)
	for _, f := range zr.File {
		name := strings.ToUpper(path.Clean(strings.ReplaceAll(f.Name, "\\", "/")))
		if f.FileInfo().IsDir() || name == "1ST_READ.BIN" || strings.HasPrefix(name, "..") || strings.HasPrefix(name, "/") || strings.Contains(name, "/../") {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
		b, err := io.ReadAll(io.LimitReader(rc, 8<<20))
		rc.Close()
		if err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
		p := filepath.Join(tmp, filepath.FromSlash(name))
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			os.RemoveAll(tmp)
			return "", err
		}
	}
	os.WriteFile(filepath.Join(tmp, openMenuFilesDone), []byte(headerLogoZipSHA), 0o644)
	os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	return dir, nil
}

// fetchHeaderLogo returns openMenu's USA theme picture.
func fetchHeaderLogo() ([]byte, error) {
	if b, err := os.ReadFile(headerLogoCache()); err == nil && len(b) > 16 {
		return b, nil
	}
	dir, err := fetchOpenMenuFiles()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(strings.ToUpper(headerLogoInZip))))
	if err != nil {
		return nil, errors.New("openMenu's release has no USA theme picture")
	}
	os.MkdirAll(filepath.Dir(headerLogoCache()), 0o755)
	os.WriteFile(headerLogoCache(), b, 0o644)
	return b, nil
}

// placeOnDisc writes b at rel inside data, keeping the spelling of folders that already exist there.
func placeOnDisc(data, rel string, b []byte) error {
	parts := strings.Split(rel, "/")
	cur := data
	for _, part := range parts[:len(parts)-1] {
		if p := findCI(cur, part); p != "" {
			cur = p
		} else {
			cur = filepath.Join(cur, part)
		}
	}
	if err := os.MkdirAll(cur, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cur, parts[len(parts)-1]), b, 0o644)
}

// addHeaderLogo makes sure the menu disc being built has the picture SWIRL takes its header logo from.
func addHeaderLogo(data string, log Logger) {
	if findCI(data, headerLogoOnDisc) != "" {
		return
	}
	b, err := fetchHeaderLogo()
	if err != nil {
		log("The Sega Dreamcast logo for the menu's header was not added (%v); SWIRL shows its name there instead. It is added at the next Update SWIRL with an internet connection.", err)
		return
	}
	if err := placeOnDisc(data, headerLogoOnDisc, b); err != nil {
		log("The Sega Dreamcast logo could not be added: %v", err)
		return
	}
	log("Added the Sega Dreamcast logo for the menu's header (from openMenu's theme)")
}

// addOpenMenuFiles adds the openMenu files a menu disc is missing: the header logo, and the theme pictures
// and fonts the Classic styles need. Files already on the disc are left as they are.
func addOpenMenuFiles(data string, log Logger) {
	missing := 0
	for _, rel := range classicFiles {
		if findCI(data, rel) == "" {
			missing++
		}
	}
	if missing == 0 {
		return
	}
	dir, err := fetchOpenMenuFiles()
	if err != nil {
		addHeaderLogo(data, log) // an earlier version may have kept the logo on its own
		log("openMenu's theme files were not added (%v), so the Classic menu styles are not available on this card. They are added at the next Update SWIRL with an internet connection.", err)
		return
	}
	var rels []string
	filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() != openMenuFilesDone {
			r, _ := filepath.Rel(dir, p)
			rels = append(rels, filepath.ToSlash(r))
		}
		return nil
	})
	sort.Strings(rels)
	added := 0
	logo := findCI(data, headerLogoOnDisc) == ""
	for _, rel := range rels {
		if findCI(data, rel) != "" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err == nil {
			err = placeOnDisc(data, rel, b)
		}
		if err != nil {
			log("openMenu's file %s could not be added: %v", rel, err)
			continue
		}
		added++
	}
	if logo && findCI(data, headerLogoOnDisc) != "" {
		log("Added the Sega Dreamcast logo for the menu's header (from openMenu's theme)")
	}
	if added > 0 {
		log("Added %d of openMenu's theme files, so the Classic menu styles work on this card", added)
	}
}

// hasClassicFiles reports whether a menu disc folder has everything the Classic styles need.
func hasClassicFiles(data string) bool {
	for _, rel := range classicFiles {
		if findCI(data, rel) == "" {
			return false
		}
	}
	return true
}
