package main

// Checking GitHub for a newer SWIRL Card Manager, and installing it.
//
// Releases are published on GitHub (see .github/workflows/release.yml). Each release is tagged with the
// version (v2.11.0) and carries SWIRL-Card-Manager.exe and SHA256SUMS.txt. The SWIRL menu is built into
// the exe, so updating Card Manager also brings the newest menu; Update SWIRL then puts it on the card.
//
// A fork's own release workflow sets updateRepo to that fork (-ldflags "-X main.updateRepo=owner/name"),
// so a fork's builds look for the fork's releases.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	updateRepo    = "TheGlengineer/SWIRL"
	updateAPIBase = "https://api.github.com"
	updateAsset   = platformAsset()
)

// platformAsset is the release file this platform installs from.
func platformAsset() string {
	if runtime.GOOS == "darwin" {
		return "SWIRL-Card-Manager-macOS.zip"
	}
	return "SWIRL-Card-Manager.exe"
}

func init() {
	if r := os.Getenv("SWIRL_UPDATE_REPO"); r != "" {
		updateRepo = r
	}
	if a := os.Getenv("SWIRL_UPDATE_API"); a != "" { // for testing against a local server
		updateAPIBase = strings.TrimRight(a, "/")
	}
}

type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest,omitempty"`
	Newer     bool   `json:"newer"`
	Name      string `json:"name,omitempty"`
	Notes     string `json:"notes,omitempty"`
	Page      string `json:"page,omitempty"` // the release page on GitHub
	Published string `json:"published,omitempty"`
	Size      int64  `json:"size,omitempty"`
	CanApply  bool   `json:"canApply"` // an exe is attached and this PC can run it
	Checked   string `json:"checked,omitempty"`
	Error     string `json:"error,omitempty"`
	Repo      string `json:"repo"`

	assetURL string
	sha256   string
}

var (
	updateMu   sync.Mutex
	lastUpdate *UpdateInfo
)

type ghAsset struct {
	Name   string `json:"name"`
	URL    string `json:"browser_download_url"`
	Size   int64  `json:"size"`
	Digest string `json:"digest"` // "sha256:..." on newer GitHub releases
}

type ghRelease struct {
	Tag       string    `json:"tag_name"`
	Name      string    `json:"name"`
	Body      string    `json:"body"`
	Page      string    `json:"html_url"`
	Draft     bool      `json:"draft"`
	Pre       bool      `json:"prerelease"`
	Published string    `json:"published_at"`
	Assets    []ghAsset `json:"assets"`
}

var updateClient = &http.Client{Timeout: 20 * time.Second}

func ghGet(url string) (*http.Response, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SWIRL-Card-Manager/"+version)
	return updateClient.Do(req)
}

func versionFromTag(tag string) string {
	return strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(tag), "v"), "V")
}

// CheckUpdate asks GitHub for the latest release. Without force, a result from the last 20 hours is reused.
func CheckUpdate(force bool) *UpdateInfo {
	updateMu.Lock()
	defer updateMu.Unlock()
	if !force && lastUpdate != nil && lastUpdate.Error == "" {
		if t, err := time.Parse(time.RFC3339, lastUpdate.Checked); err == nil && time.Since(t) < 20*time.Hour {
			return lastUpdate
		}
	}
	u := &UpdateInfo{Current: version, Repo: updateRepo, Checked: time.Now().Format(time.RFC3339)}
	lastUpdate = u
	resp, err := ghGet(updateAPIBase + "/repos/" + updateRepo + "/releases/latest")
	if err != nil {
		u.Error = "GitHub could not be reached. Check the internet connection and try again."
		return u
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		u.Error = "No releases have been published yet."
		return u
	case resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests:
		u.Error = "GitHub is limiting checks from this connection for now. Try again in an hour."
		return u
	case resp.StatusCode != http.StatusOK:
		u.Error = fmt.Sprintf("GitHub answered %s.", resp.Status)
		return u
	}
	var rel ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&rel); err != nil {
		u.Error = "GitHub's answer could not be read."
		return u
	}
	u.Latest = versionFromTag(rel.Tag)
	u.Name, u.Notes, u.Page, u.Published = rel.Name, rel.Body, rel.Page, rel.Published
	u.Newer = u.Latest != "" && versionNewer(u.Latest, version)
	var sums string
	for _, a := range rel.Assets {
		switch {
		case strings.EqualFold(a.Name, updateAsset):
			u.assetURL, u.Size = a.URL, a.Size
			if d, ok := strings.CutPrefix(a.Digest, "sha256:"); ok {
				u.sha256 = strings.ToLower(d)
			}
		case strings.EqualFold(a.Name, "SHA256SUMS.txt"):
			sums = a.URL
		}
	}
	if u.sha256 == "" && sums != "" {
		u.sha256 = shaFromSums(sums, updateAsset)
	}
	u.CanApply = u.Newer && u.assetURL != "" && u.sha256 != "" && canSelfUpdate
	return u
}

// shaFromSums reads "hash  name" lines (sha256sum output).
func shaFromSums(url, name string) string {
	resp, err := ghGet(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.EqualFold(strings.TrimPrefix(f[1], "*"), name) && len(f[0]) == 64 {
			return strings.ToLower(f[0])
		}
	}
	return ""
}

func updatesDir() string { return filepath.Join(appDataDir(), "updates") }

// downloadUpdate fetches the new exe and checks it against the published SHA-256.
func downloadUpdate(u *UpdateInfo) (string, error) {
	if err := os.MkdirAll(updatesDir(), 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(updatesDir(), fmt.Sprintf("SWIRL-Card-Manager-%s%s", u.Latest, filepath.Ext(updateAsset)))
	tmp := dst + ".part"
	req, _ := http.NewRequest("GET", u.assetURL, nil)
	req.Header.Set("User-Agent", "SWIRL-Card-Manager/"+version)
	resp, err := (&http.Client{Timeout: 15 * time.Minute}).Do(req)
	if err != nil {
		return "", errors.New("the download could not start; check the internet connection")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("the download failed (%s)", resp.Status)
	}
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	total := u.Size
	if total <= 0 {
		total = resp.ContentLength
	}
	var done int64
	buf := make([]byte, 256<<10)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			f.Write(buf[:n])
			h.Write(buf[:n])
			done += int64(n)
			if total > 0 {
				setPct(float64(done) / float64(total))
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmp)
			return "", errors.New("the download stopped part way; try again")
		}
	}
	f.Close()
	if got := hex.EncodeToString(h.Sum(nil)); got != u.sha256 {
		os.Remove(tmp)
		return "", errors.New("the downloaded file does not match the published checksum, so it was not used")
	}
	os.Remove(dst)
	if err := os.Rename(tmp, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// StartUpdate downloads the latest release and hands over to it.
func StartUpdate() error {
	u := CheckUpdate(true)
	if u.Error != "" {
		return errors.New(u.Error)
	}
	if !u.Newer {
		return errors.New("this is already the newest version")
	}
	if !u.CanApply {
		return errors.New("this release cannot be installed from here; download it from its GitHub page")
	}
	return runJob("Downloading SWIRL Card Manager "+u.Latest, "", func() error {
		jobLog("Downloading version %s from github.com/%s", u.Latest, updateRepo)
		exe, err := downloadUpdate(u)
		if err != nil {
			return err
		}
		jobLog("Checksum verified.")
		jobLog("Installing version %s. The app restarts in its own window in a few seconds.", u.Latest)
		return handOverToUpdate(exe)
	})
}

// cleanUpdates removes downloaded updates once a newer copy is running from somewhere else.
func cleanUpdates() {
	self, _ := os.Executable()
	if strings.HasPrefix(strings.ToLower(filepath.Clean(self)), strings.ToLower(updatesDir())) {
		return
	}
	os.RemoveAll(updatesDir())
}
