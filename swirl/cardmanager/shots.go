package main

// Screenshots for SWIRL's game detail screen. Two pictures per game, stored on the card in SWIRL/shots
// by serial and packed into SHOT.DAT on the menu disc (two 256x256 RGB565 PVR textures per game, the
// 4:3 picture on the top 192 lines). "Get screenshots online" pulls a title screen and an in-game shot
// from the libretro-thumbnails Dreamcast collection on GitHub, matched by game name.

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	shotPVRBytes = 32 + 256*256*2
	shotChunk    = 2 * shotPVRBytes
	thumbRepo    = "libretro-thumbnails/Sega_-_Dreamcast"
)

func shotPath(root, product string, n int) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(product)
	return filepath.Join(root, editsDir, "shots", fmt.Sprintf("%s_%d.png", safe, n))
}

func shotsFor(root, product string) []string {
	var out []string
	for n := 1; n <= 2; n++ {
		if p := shotPath(root, product, n); fileExists(p) {
			out = append(out, p)
		}
	}
	return out
}

// saveShot stores a picture as a 256x192 PNG (the size SWIRL shows).
func saveShot(root, product string, n int, img image.Image) error {
	os.MkdirAll(filepath.Join(root, editsDir, "shots"), 0o755)
	small := resample(img, img.Bounds(), 256, 192)
	return os.WriteFile(shotPath(root, product, n), pngBytes(small), 0o644)
}

func shotPVR(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return nil
	}
	canvas := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	draw.Draw(canvas, canvas.Bounds(), image.Black, image.Point{}, draw.Src)
	draw.Draw(canvas, image.Rect(0, 0, 256, 192), resample(img, img.Bounds(), 256, 192), image.Point{}, draw.Src)
	return encodePVR565(canvas, 256)
}

// buildShotDat writes SHOT.DAT into the menu data folder (or removes it when no game has pictures).
func buildShotDat(root string, c *Card, data string, log Logger) error {
	d := newDat(shotChunk)
	n := 0
	seen := map[string]bool{}
	for _, g := range c.Games {
		if g.Product == "" || seen[g.Product] {
			continue
		}
		seen[g.Product] = true
		paths := shotsFor(root, g.Product)
		if len(paths) == 0 {
			continue
		}
		chunk := make([]byte, shotChunk)
		k := 0
		for _, p := range paths {
			if b := shotPVR(p); len(b) == shotPVRBytes {
				copy(chunk[k*shotPVRBytes:], b)
				k++
			}
		}
		if k == 0 {
			continue
		}
		d.Set(g.Product, chunk)
		n++
	}
	for _, e := range listDir(data) {
		if strings.EqualFold(e, "SHOT.DAT") {
			os.Remove(filepath.Join(data, e))
		}
	}
	if n == 0 {
		return nil
	}
	if err := d.Write(filepath.Join(data, "SHOT.DAT")); err != nil {
		return err
	}
	log("Added screenshots for %d games", n)
	return nil
}

func listDir(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// ---------- matching game names to the libretro collection ----------

var (
	parenRe = regexp.MustCompile(`\s*\(.*$`)
	romans  = map[string]string{"i": "1", "ii": "2", "iii": "3", "iv": "4", "v": "5", "vi": "6"}
)

func normTitle(s string) string {
	s = strings.ToLower(s)
	// "House of the Dead 2, The" -> "the house of the dead 2"
	if i := strings.Index(s, ", the"); i >= 0 {
		s = "the " + s[:i] + s[i+5:]
	}
	s = strings.NewReplacer("&", " and ", "'", "", "-", " ", ":", " ", ".", " ", ",", " ").Replace(s)
	var b strings.Builder
	for _, w := range strings.Fields(s) {
		if r, ok := romans[w]; ok {
			w = r
		}
		b.WriteString(w)
	}
	return b.String()
}

// disc names that are abbreviations of the real title
var titleAliases = map[string]string{
	"vf3tb": "virtuafighter3tb", "segagt": "segagt", "msr": "metropolisstreetracer", "ssx": "ssx",
	"daytonausa2001": "daytonausa2001", "sa2": "sonicadventure2", "powerstone2usa": "powerstone2",
	"rainbowsix": "tomclancysrainbowsixwitheaglewatchmissions", "roguespear": "tomclancysrainbowsixroguespear",
}

type thumbEntry struct {
	file, base, main string // file name, title without regions, title before " - "
}

func regionRank(file string) int {
	switch {
	case strings.Contains(file, "(USA)"):
		return 0
	case strings.Contains(file, "(USA,") || strings.Contains(file, ", USA"):
		return 1
	case strings.Contains(file, "(Europe"):
		return 2
	case strings.Contains(file, "(Japan"):
		return 4
	}
	return 3
}

// bestThumb picks the libretro file whose title best matches a game name, or "".
func bestThumb(name string, list []thumbEntry) string {
	want := normTitle(name)
	if a, ok := titleAliases[want]; ok {
		want = a
	}
	if want == "" {
		return ""
	}
	best, bestScore := "", 0
	for _, e := range list {
		score := 0
		switch {
		case e.base == want:
			score = 100
		case e.main == want:
			score = 92
		case strings.HasPrefix(want, e.base) && len(e.base) >= 5:
			score = 70 + 20*len(e.base)/len(want)
		case strings.HasPrefix(e.base, want) && len(want) >= 5:
			score = 60 + 20*len(want)/len(e.base)
		case strings.HasPrefix(e.main, want) && len(want) >= 5:
			score = 55 + 20*len(want)/len(e.main)
		case len(want) >= 8 && strings.Contains(e.base, want):
			score = 40 + 20*len(want)/len(e.base) // "Tom Clancy's Rainbow Six" for RAINBOW SIX
		}
		if score == 0 {
			continue
		}
		score = score*10 - regionRank(e.file)
		if strings.Contains(e.file, "(Beta") || strings.Contains(e.file, "(Proto") || strings.Contains(e.file, "(Demo") {
			score -= 50
		}
		if score > bestScore {
			best, bestScore = e.file, score
		}
	}
	return best
}

var thumbClient = &http.Client{Timeout: 60 * time.Second}

// thumbIndex lists the pictures in the libretro collection (one GitHub API call, cached for a week).
func thumbIndex() (map[string][]thumbEntry, error) {
	cache := filepath.Join(dbDir(), "thumbs.json")
	var tree struct {
		Tree []struct{ Path string } `json:"tree"`
	}
	b, err := os.ReadFile(cache)
	if st, e := os.Stat(cache); err != nil || e != nil || time.Since(st.ModTime()) > 7*24*time.Hour {
		resp, err := thumbClient.Get("https://api.github.com/repos/" + thumbRepo + "/git/trees/master?recursive=1")
		if err != nil {
			return nil, fmt.Errorf("could not reach GitHub (%v)", err)
		}
		b, err = io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			return nil, fmt.Errorf("GitHub did not send the screenshot list (%s)", resp.Status)
		}
		os.MkdirAll(dbDir(), 0o755)
		os.WriteFile(cache, b, 0o644)
	}
	if err := json.Unmarshal(b, &tree); err != nil {
		return nil, errors.New("the screenshot list from GitHub could not be read")
	}
	out := map[string][]thumbEntry{}
	for _, t := range tree.Tree {
		dir, file, ok := strings.Cut(t.Path, "/")
		if !ok || !strings.HasSuffix(strings.ToLower(file), ".png") {
			continue
		}
		title := strings.TrimSuffix(file, ".png")
		base := parenRe.ReplaceAllString(title, "")
		main := base
		if i := strings.Index(base, " - "); i > 0 {
			main = base[:i]
		}
		out[dir] = append(out[dir], thumbEntry{file: file, base: normTitle(base), main: normTitle(main)})
	}
	for k := range out {
		sort.Slice(out[k], func(i, j int) bool { return out[k][i].file < out[k][j].file })
	}
	return out, nil
}

func fetchThumb(dir, file string) (image.Image, error) {
	u := "https://raw.githubusercontent.com/" + thumbRepo + "/master/" + url.PathEscape(dir) + "/" + url.PathEscape(file)
	resp, err := thumbClient.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New(resp.Status)
	}
	img, _, err := image.Decode(resp.Body)
	return img, err
}

// StartShotDownload fills in screenshots for games that have none.
func StartShotDownload(root string) error {
	c, err := ScanCard(root)
	if err != nil {
		return err
	}
	return runJob("Getting screenshots", "", func() error {
		jobLog("Reading the libretro Dreamcast picture list")
		idx, err := thumbIndex()
		if err != nil {
			return err
		}
		var todo []Game
		seen := map[string]bool{}
		for _, g := range c.Games {
			if g.Product == "" || seen[g.Product] || len(shotsFor(root, g.Product)) > 0 {
				continue
			}
			seen[g.Product] = true
			todo = append(todo, g)
		}
		got, missing := 0, []string{}
		for i, g := range todo {
			jobUpdate(func(j *jobState) {
				j.Stage = fmt.Sprintf("%s (%d of %d)", g.Name, i+1, len(todo))
				j.Pct = 0.95 * float64(i) / float64(len(todo))
			})
			n := 0
			for _, dir := range []string{"Named_Titles", "Named_Snaps"} {
				f := bestThumb(g.Name, idx[dir])
				if f == "" {
					continue
				}
				img, err := fetchThumb(dir, f)
				if err != nil {
					continue
				}
				if saveShot(root, g.Product, n+1, img) == nil {
					n++
				}
			}
			if n > 0 {
				got++
			} else {
				missing = append(missing, g.Name)
			}
		}
		jobLog("Added screenshots for %d of %d games. Click Update SWIRL to put them on the card.", got, len(todo))
		if len(missing) > 0 {
			jobLog("No match for: %s. Rename a game or add pictures in its Edit window.", strings.Join(missing, ", "))
		}
		return nil
	})
}

type ShotResult struct {
	Count   int      `json:"count"`
	Matches []string `json:"matches"` // the libretro pictures that were used
}

// FetchGameShots gets the title screen and an in-game picture for one game, from the Edit window. The
// names are tried in order (the name typed in the window first, then the card's), then the game's
// official title from its serial. Any screenshots the game had are replaced, but only when something
// was found.
func FetchGameShots(root, product string, names []string) (ShotResult, error) {
	var r ShotResult
	if product == "" {
		return r, errors.New("this game has no serial, so screenshots cannot be stored for it")
	}
	if e, ok := lookupTitle(product, 0); ok {
		names = append(names, e.Title)
	}
	var tried []string
	for _, n := range names {
		if n = strings.TrimSpace(n); n != "" {
			tried = append(tried, n)
		}
	}
	names = dedupStrings(tried)
	idx, err := thumbIndex()
	if err != nil {
		return r, err
	}
	type pick struct {
		dir, file string
	}
	var picks []pick
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		picks = picks[:0]
		for _, dir := range []string{"Named_Titles", "Named_Snaps"} {
			if f := bestThumb(name, idx[dir]); f != "" {
				picks = append(picks, pick{dir, f})
			}
		}
		if len(picks) > 0 {
			break
		}
	}
	if len(picks) == 0 {
		return r, fmt.Errorf("no screenshots found online for %s; try a different name, or upload pictures", strings.Join(names, " / "))
	}
	var imgs []image.Image
	for _, p := range picks {
		img, err := fetchThumb(p.dir, p.file)
		if err != nil {
			return r, fmt.Errorf("could not download %s (%v)", p.file, err)
		}
		imgs = append(imgs, img)
		r.Matches = append(r.Matches, strings.TrimSuffix(p.file, ".png"))
	}
	for n := 1; n <= 2; n++ {
		os.Remove(shotPath(root, product, n))
	}
	for i, img := range imgs {
		if err := saveShot(root, product, i+1, img); err != nil {
			return r, err
		}
		r.Count++
	}
	return r, nil
}
