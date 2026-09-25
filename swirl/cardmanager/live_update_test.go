package main

import (
	"os"
	"testing"
)

// SWIRL_LIVE_UPDATE=1 go test -run TestLiveUpdate checks the real latest release on GitHub: it must be
// found, and its exe must download and match the published checksum.
func TestLiveUpdate(t *testing.T) {
	if os.Getenv("SWIRL_LIVE_UPDATE") == "" {
		t.Skip("set SWIRL_LIVE_UPDATE=1 to check the real GitHub release")
	}
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", os.Getenv("LOCALAPPDATA"))
	u := CheckUpdate(true)
	t.Logf("repo %s, this version %s, latest %s, newer %v, size %d, sha256 %s", u.Repo, u.Current, u.Latest, u.Newer, u.Size, u.sha256)
	if u.Error != "" || u.Latest == "" || u.sha256 == "" || u.assetURL == "" {
		t.Fatalf("release not usable: %+v", u)
	}
	p, err := downloadUpdate(u)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := os.Stat(p)
	t.Logf("downloaded and verified %s (%d bytes)", p, st.Size())
}
