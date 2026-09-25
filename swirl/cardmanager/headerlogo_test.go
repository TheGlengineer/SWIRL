package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
