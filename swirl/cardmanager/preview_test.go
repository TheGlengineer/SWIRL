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

func TestVersionOrder(t *testing.T) {
	newer := [][2]string{
		{"2.14.0-preview.1", "2.13.3"},
		{"2.14.0", "2.14.0-preview.1"},
		{"2.14.0", "2.14.0-preview.9"},
		{"2.14.0-preview.2", "2.14.0-preview.1"},
		{"2.14.0-preview.10", "2.14.0-preview.9"},
		{"2.14.1-preview.1", "2.14.0"},
		{"2.10", "2.9"},
		{"2.13.3", "2.13.2"},
	}
	for _, p := range newer {
		if !versionNewer(p[0], p[1]) || versionNewer(p[1], p[0]) {
			t.Errorf("%s should be newer than %s", p[0], p[1])
		}
	}
	for _, v := range []string{"2.14.0-preview.1", "2.13.3", "2.14.0"} {
		if versionNewer(v, v) {
			t.Errorf("%s newer than itself", v)
		}
	}
	if versionLabel("2.14.0-preview.1") != "2.14.0 preview 1" || versionLabel("2.13.3") != "2.13.3" || versionLabel("2.17.0-beta.1") != "2.17.0 beta 1" {
		t.Error("labels")
	}
	if !isPreview("2.14.0-preview.1") || isPreview("2.14.0") {
		t.Error("isPreview")
	}
}

// a GitHub with a latest release and a list of releases, some of them previews
func fakeGitHubWithPreviews(t *testing.T, latest string, list []string, exe []byte) *httptest.Server {
	sum := sha256.Sum256(exe)
	hexsum := hex.EncodeToString(sum[:])
	var srv *httptest.Server
	rel := func(tag string) string {
		pre := strings.Contains(tag, "-")
		return fmt.Sprintf(`{"tag_name":%q,"name":"SWIRL %s","body":"notes for %s","prerelease":%v,"draft":false,"html_url":"https://example/%s","published_at":"2026-10-01T12:00:00Z","assets":[{"name":%q,"digest":"sha256:%s","size":%d,"browser_download_url":"%s/dl/%s"}]}`,
			tag, tag, tag, pre, tag, updateAsset, hexsum, len(exe), srv.URL, tag)
	}
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/owner/swirl/releases/latest":
			fmt.Fprint(w, rel(latest))
		case r.URL.Path == "/repos/owner/swirl/releases":
			parts := []string{}
			for _, tag := range list {
				parts = append(parts, rel(tag))
			}
			fmt.Fprint(w, "["+strings.Join(parts, ",")+"]")
		case strings.HasPrefix(r.URL.Path, "/dl/"):
			w.Write(exe)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestPreviewOffer(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", os.Getenv("LOCALAPPDATA"))
	t.Setenv("HOME", os.Getenv("LOCALAPPDATA"))
	oldRepo, oldBase, oldVer := updateRepo, updateAPIBase, appVersion
	defer func() { updateRepo, updateAPIBase, appVersion, lastUpdate = oldRepo, oldBase, oldVer, nil }()
	updateRepo = "owner/swirl"
	exe := []byte("pretend exe " + strings.Repeat("y", 3000))
	list := []string{"v2.14.0-preview.2", "v2.14.0-preview.1", "v2.13.3", "v2.13.2"}

	// on the latest release: the newest preview is offered, the normal update is not
	appVersion = "2.13.3"
	updateAPIBase = fakeGitHubWithPreviews(t, "v2.13.3", list, exe).URL
	u := CheckUpdate(true)
	if u.Error != "" || u.Newer || u.OnPreview || u.Preview == nil || u.Preview.Version != "2.14.0-preview.2" ||
		u.Preview.Label != "2.14.0 preview 2" || u.Preview.Notes != "notes for v2.14.0-preview.2" {
		t.Fatalf("offer: %+v %+v", u, u.Preview)
	}
	if u.Preview.CanApply != canSelfUpdate {
		t.Fatal("preview canApply")
	}

	// turned off in About: no offer
	off := false
	SaveUIPrefs(UIPrefs{PreviewOffers: &off})
	if u := CheckUpdate(true); u.Preview != nil {
		t.Fatal("offered although turned off")
	}
	SaveUIPrefs(UIPrefs{})

	// an older version that the normal update covers: the normal update, and still the preview
	appVersion = "2.13.2"
	u = CheckUpdate(true)
	if !u.Newer || u.Latest != "2.13.3" || u.Preview == nil {
		t.Fatalf("older copy: %+v", u)
	}

	// on preview 1: preview 2 is offered, and going back to 2.13.3 is possible
	appVersion = "2.14.0-preview.1"
	u = CheckUpdate(true)
	if u.Newer || !u.OnPreview || u.Preview == nil || u.Preview.Version != "2.14.0-preview.2" || u.CanGoBack != canSelfUpdate {
		t.Fatalf("on preview: %+v", u)
	}

	// on the newest preview: nothing newer to offer
	appVersion = "2.14.0-preview.2"
	if u := CheckUpdate(true); u.Preview != nil || u.Newer {
		t.Fatalf("newest preview: %+v", u)
	}

	// the release comes out: previews are older now, and the normal update takes the preview user there
	updateAPIBase = fakeGitHubWithPreviews(t, "v2.14.0", append([]string{"v2.14.0"}, list...), exe).URL
	u = CheckUpdate(true)
	if !u.Newer || u.Latest != "2.14.0" || u.Preview != nil {
		t.Fatalf("after release: %+v", u)
	}

	// no previews at all
	appVersion = "2.13.3"
	updateAPIBase = fakeGitHubWithPreviews(t, "v2.13.3", []string{"v2.13.3", "v2.13.2"}, exe).URL
	if u := CheckUpdate(true); u.Preview != nil || u.Newer {
		t.Fatalf("no previews: %+v", u)
	}
}

func TestPreviewDownload(t *testing.T) {
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", os.Getenv("LOCALAPPDATA"))
	t.Setenv("HOME", os.Getenv("LOCALAPPDATA"))
	oldRepo, oldBase, oldVer := updateRepo, updateAPIBase, appVersion
	defer func() { updateRepo, updateAPIBase, appVersion, lastUpdate = oldRepo, oldBase, oldVer, nil }()
	updateRepo = "owner/swirl"
	exe := []byte("pretend preview exe " + strings.Repeat("z", 3000))
	appVersion = "2.13.3"
	updateAPIBase = fakeGitHubWithPreviews(t, "v2.13.3", []string{"v2.14.0-preview.1", "v2.13.3"}, exe).URL
	u := CheckUpdate(true)
	p := u.Preview
	if p == nil {
		t.Fatal("no preview")
	}
	path, err := download(p.Version, p.assetURL, p.Size, p.sha256)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(exe) || !strings.Contains(path, "2.14.0-preview.1") {
		t.Fatalf("downloaded %s", path)
	}
}
