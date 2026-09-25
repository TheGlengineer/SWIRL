package main

// The Sega Dreamcast logo at the top left of SWIRL's screens comes from openMenu's USA theme picture
// (THEME/NTSC_U/BG_U_L.PVR) on the menu disc. Cards set up with GDMENUCardManager already have it. For
// cards that do not (a new card, or one from another tool), Card Manager fetches that one picture from
// openMenu's own release on GitHub, keeps it with the app's data and puts it on the menu disc. It is not
// stored in this repository. Without it, SWIRL shows its name instead.

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	headerLogoZipURL = "https://github.com/mrneo240/openMenu/releases/download/2021-12-4_alpha2/openmenu_12-5-2021_alpha2.zip"
	headerLogoZipSHA = "1a606c2118a1a76dbf86de849e210ee8d3fe54d7dc0fbf242c0cdaae9c5ecac8"
	headerLogoInZip  = "theme/ntsc_u/bg_u_l.pvr"
)

const headerLogoOnDisc = "THEME/NTSC_U/BG_U_L.PVR"

func headerLogoCache() string { return filepath.Join(appDataDir(), "openmenu", "BG_U_L.PVR") }

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

// fetchHeaderLogo downloads openMenu's release once and keeps the USA theme picture from it.
func fetchHeaderLogo() ([]byte, error) {
	if b, err := os.ReadFile(headerLogoCache()); err == nil && len(b) > 16 {
		return b, nil
	}
	if headerLogoZipURL == "" {
		return nil, errors.New("downloads are off")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	req, _ := http.NewRequest("GET", headerLogoZipURL, nil)
	req.Header.Set("User-Agent", "SWIRL-Card-Manager/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("openMenu's release could not be reached")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openMenu's release answered %s", resp.Status)
	}
	zb, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(zb)
	if hex.EncodeToString(sum[:]) != headerLogoZipSHA {
		return nil, errors.New("openMenu's release did not match the expected file")
	}
	zr, err := zip.NewReader(bytes.NewReader(zb), int64(len(zb)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if !strings.EqualFold(f.Name, headerLogoInZip) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		os.MkdirAll(filepath.Dir(headerLogoCache()), 0o755)
		os.WriteFile(headerLogoCache(), b, 0o644)
		return b, nil
	}
	return nil, errors.New("openMenu's release has no USA theme picture")
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
	dst := filepath.Join(data, filepath.FromSlash(headerLogoOnDisc))
	if p := findCI(data, "THEME"); p != "" { // keep the folder's existing spelling
		if q := findCI(p, "NTSC_U"); q != "" {
			dst = filepath.Join(q, "BG_U_L.PVR")
		} else {
			dst = filepath.Join(p, "NTSC_U", "BG_U_L.PVR")
		}
	}
	os.MkdirAll(filepath.Dir(dst), 0o755)
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		log("The Sega Dreamcast logo could not be added: %v", err)
		return
	}
	log("Added the Sega Dreamcast logo for the menu's header (from openMenu's theme)")
}
