package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func fakeGitHub(t *testing.T, tag string, exe []byte, digest bool) *httptest.Server {
	sum := sha256.Sum256(exe)
	hexsum := hex.EncodeToString(sum[:])
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/owner/swirl/releases/latest":
			d := ""
			if digest {
				d = `"digest":"sha256:` + hexsum + `",`
			}
			fmt.Fprintf(w, `{"tag_name":%q,"name":"SWIRL %s","body":"- New things","html_url":"https://github.com/owner/swirl/releases/tag/%s","published_at":"2026-10-01T12:00:00Z","assets":[
				{"name":"SWIRL-Card-Manager.exe",%s"size":%d,"browser_download_url":"%s/dl/exe"},
				{"name":"SHA256SUMS.txt","size":90,"browser_download_url":"%s/dl/sums"}]}`, tag, tag, tag, d, len(exe), srv.URL, srv.URL)
		case "/dl/exe":
			w.Write(exe)
		case "/dl/sums":
			fmt.Fprintf(w, "%s  SWIRL-Card-Manager.exe\n", hexsum)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestUpdateCheck(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", os.Getenv("LOCALAPPDATA"))
	oldRepo, oldBase := updateRepo, updateAPIBase
	defer func() { updateRepo, updateAPIBase, lastUpdate = oldRepo, oldBase, nil }()
	updateRepo = "owner/swirl"
	exe := []byte("pretend exe " + strings.Repeat("x", 5000))

	for _, digest := range []bool{true, false} {
		srv := fakeGitHub(t, "v99.0.1", exe, digest)
		updateAPIBase = srv.URL
		u := CheckUpdate(true)
		if u.Error != "" || !u.Newer || u.Latest != "99.0.1" || u.sha256 == "" || u.Notes != "- New things" {
			t.Fatalf("digest=%v: %+v", digest, u)
		}
		if u.CanApply != canSelfUpdate {
			t.Fatal("canApply")
		}
		p, err := downloadUpdate(u)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(p)
		if string(b) != string(exe) {
			t.Fatal("download differs")
		}
		// a tampered file is refused
		u.sha256 = strings.Repeat("0", 64)
		if _, err := downloadUpdate(u); err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Fatal("bad checksum accepted:", err)
		}
	}
	// same version: not newer
	srv := fakeGitHub(t, "v"+version, exe, true)
	updateAPIBase = srv.URL
	if u := CheckUpdate(true); u.Newer || u.Error != "" {
		t.Fatalf("same version: %+v", u)
	}
	// cached within the day unless forced
	updateAPIBase = "http://127.0.0.1:1"
	if u := CheckUpdate(false); u.Error != "" {
		t.Fatal("cache not used", u.Error)
	}
	if u := CheckUpdate(true); u.Error == "" {
		t.Fatal("unreachable GitHub not reported")
	}
	// no releases yet
	srv404 := httptest.NewServer(http.NotFoundHandler())
	defer srv404.Close()
	updateAPIBase = srv404.URL
	if u := CheckUpdate(true); !strings.Contains(u.Error, "No releases") {
		t.Fatal(u.Error)
	}
}
