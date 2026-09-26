package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHeaderLogo(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", os.Getenv("LOCALAPPDATA"))
	t.Setenv("HOME", os.Getenv("LOCALAPPDATA")) // macOS keeps its cache under HOME
	pvr := []byte("GBIX fake theme picture " + strings.Repeat("x", 100))
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("theme/ntsc_u/bg_u_l.pvr")
	w.Write(pvr)
	zw.Close()
	sum := sha256.Sum256(zb.Bytes())
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { hits++; rw.Write(zb.Bytes()) }))
	oldURL, oldSHA := headerLogoZipURL, headerLogoZipSHA
	defer func() { headerLogoZipURL, headerLogoZipSHA = oldURL, oldSHA }()
	headerLogoZipURL, headerLogoZipSHA = srv.URL, hex.EncodeToString(sum[:])
	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, f) }

	// a disc with no theme gets the picture
	data := t.TempDir()
	addHeaderLogo(data, logf)
	b, err := os.ReadFile(filepath.Join(data, "THEME", "NTSC_U", "BG_U_L.PVR"))
	if err != nil || !bytes.Equal(b, pvr) {
		t.Fatal("picture not added", err, logs)
	}
	// a disc that has it (any case) is left alone
	data2 := t.TempDir()
	os.MkdirAll(filepath.Join(data2, "theme", "ntsc_u"), 0o755)
	os.WriteFile(filepath.Join(data2, "theme", "ntsc_u", "bg_u_l.pvr"), []byte("mine"), 0o644)
	addHeaderLogo(data2, logf)
	if b, _ := os.ReadFile(filepath.Join(data2, "theme", "ntsc_u", "bg_u_l.pvr")); string(b) != "mine" {
		t.Fatal("existing theme replaced")
	}
	// the second disc comes from the cache, without downloading again
	srv.Close()
	data3 := t.TempDir()
	addHeaderLogo(data3, logf)
	if hits != 1 || findCI(data3, headerLogoOnDisc) == "" {
		t.Fatalf("cache not used: %d downloads", hits)
	}
	// a changed release is refused
	os.Remove(headerLogoCache())
	os.RemoveAll(openMenuFilesDir())
	srv2 := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.Write([]byte("something else")) }))
	defer srv2.Close()
	headerLogoZipURL = srv2.URL
	data4 := t.TempDir()
	logs = nil
	addHeaderLogo(data4, logf)
	if findCI(data4, headerLogoOnDisc) != "" || len(logs) == 0 {
		t.Fatal("a file with the wrong checksum was used")
	}
}

func fakeOpenMenuRelease(t *testing.T) (zipBytes []byte, files map[string][]byte) {
	t.Helper()
	files = map[string][]byte{}
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	add := func(name string, body []byte) {
		w, _ := zw.Create(name)
		w.Write(body)
	}
	add("1ST_READ.BIN", []byte("openMenu program, must never be used"))
	for _, rel := range append(append([]string{}, classicFiles...), "THEME/PAL/BG_E_L.PVR", "THEME/CUST_0/THEME.INI") {
		body := []byte("file " + rel + strings.Repeat(".", 40))
		files[rel] = body
		add(strings.ToLower(rel), body) // the release uses lower case names
	}
	zw.Close()
	return zb.Bytes(), files
}

func TestOpenMenuFilesForClassicStyles(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", os.Getenv("LOCALAPPDATA"))
	t.Setenv("HOME", os.Getenv("LOCALAPPDATA"))
	zb, files := fakeOpenMenuRelease(t)
	sum := sha256.Sum256(zb)
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { hits++; rw.Write(zb) }))
	defer srv.Close()
	oldURL, oldSHA := headerLogoZipURL, headerLogoZipSHA
	defer func() { headerLogoZipURL, headerLogoZipSHA = oldURL, oldSHA }()
	headerLogoZipURL, headerLogoZipSHA = srv.URL, hex.EncodeToString(sum[:])
	var logs []string
	logf := func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) }

	// a disc from GDMENU or a new card: everything is added, openMenu's program is not
	gd := t.TempDir()
	os.WriteFile(filepath.Join(gd, "1ST_READ.BIN"), []byte("SWIRL"), 0o644)
	if hasClassicFiles(gd) {
		t.Fatal("empty disc reported as having the Classic files")
	}
	addOpenMenuFiles(gd, logf)
	if !hasClassicFiles(gd) {
		t.Fatal("Classic files missing after adding", logs)
	}
	for rel, body := range files {
		b, err := os.ReadFile(findCI(gd, rel))
		if err != nil || !bytes.Equal(b, body) {
			t.Fatalf("%s not added correctly: %v", rel, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(gd, "1ST_READ.BIN")); string(b) != "SWIRL" {
		t.Fatal("openMenu's program replaced SWIRL's")
	}

	// a disc from openMenu: its own files are kept byte for byte, only missing ones are added
	om := t.TempDir()
	os.MkdirAll(filepath.Join(om, "theme", "shared"), 0o755)
	os.WriteFile(filepath.Join(om, "theme", "shared", "highlight.pvr"), []byte("the owner's own"), 0o644)
	os.WriteFile(filepath.Join(om, "EMPTY.PVR"), []byte("the owner's empty"), 0o644)
	addOpenMenuFiles(om, logf)
	if b, _ := os.ReadFile(filepath.Join(om, "theme", "shared", "highlight.pvr")); string(b) != "the owner's own" {
		t.Fatal("an existing theme file was replaced")
	}
	if b, _ := os.ReadFile(filepath.Join(om, "EMPTY.PVR")); string(b) != "the owner's empty" {
		t.Fatal("an existing EMPTY.PVR was replaced")
	}
	if p := findCI(om, "THEME/SHARED/ICON_WHITE.PVR"); p == "" || !strings.Contains(filepath.ToSlash(p), "/theme/shared/") {
		t.Fatal("missing file not added in the existing folder spelling:", p)
	}
	entries, _ := os.ReadDir(om)
	for _, e := range entries {
		if e.Name() == "THEME" {
			t.Fatal("a second THEME folder was made next to the existing theme folder")
		}
	}

	// a complete disc is not touched and nothing is downloaded again
	before := hits
	logs = nil
	addOpenMenuFiles(gd, logf)
	if hits != before || len(logs) != 0 {
		t.Fatal("a complete disc was changed or downloaded for", logs)
	}
	// the next card comes from the cache
	srv.Close()
	gd2 := t.TempDir()
	addOpenMenuFiles(gd2, logf)
	if !hasClassicFiles(gd2) || hits != 1 {
		t.Fatalf("cache not used (%d downloads)", hits)
	}

	// offline with no cache: nothing added, SWIRL still builds, and the log says why
	os.RemoveAll(openMenuFilesDir())
	os.Remove(headerLogoCache())
	headerLogoZipURL = ""
	gd3 := t.TempDir()
	logs = nil
	addOpenMenuFiles(gd3, logf)
	if hasClassicFiles(gd3) || !strings.Contains(strings.Join(logs, "\n"), "Classic menu styles are not available") {
		t.Fatal("offline case not reported", logs)
	}

	// a release that does not match the expected checksum is refused
	bad := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) { rw.Write([]byte("not openMenu")) }))
	defer bad.Close()
	headerLogoZipURL = bad.URL
	gd4 := t.TempDir()
	addOpenMenuFiles(gd4, logf)
	if findCI(gd4, "EMPTY.PVR") != "" {
		t.Fatal("files from a release with the wrong checksum were used")
	}
}
