package main

// Changes waiting for Update SWIRL (2.17). Everything the menu disc is built from lives on the card: the game
// folders, SWIRL/games.json, the art, the collections, the look and sound files, the extras. After every
// successful build Card Manager writes SWIRL/menu.json with a fingerprint of each of those groups; a scan
// fingerprints them again and the window shows which groups changed since the Dreamcast last got a menu.
// A card without the stamp (made by an older Card Manager) says nothing until its next Update SWIRL.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const menuStampName = "menu.json"

// pendingKeys are the groups, in the order the window lists them.
var pendingKeys = []string{"games", "edits", "art", "collections", "look", "extras", "version"}

type buildStamp struct {
	Built    time.Time         `json:"built"`
	Version  string            `json:"version"`  // Card Manager that built it
	Menu     string            `json:"menu"`     // build hash of the menu binary it carries
	Sections map[string]string `json:"sections"` // group -> fingerprint
}

// Pending is what the window shows on the Update SWIRL button and beside each section.
type Pending struct {
	Known    bool      `json:"known"`    // false until the card has been built by a Card Manager that stamps it
	Sections []string  `json:"sections"` // the groups that changed since the last build
	Built    time.Time `json:"built,omitempty"`
}

func menuStampPath(root string) string { return filepath.Join(root, editsDir, menuStampName) }

func fingerprint(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:8])
}

// dirListing fingerprints a folder by its file names, sizes and times: enough to notice any change without
// reading the files, which matters for the art folder of a big card.
func dirListing(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "none"
	}
	var parts []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s|%d|%d", strings.ToLower(e.Name()), info.Size(), info.ModTime().Unix()/2))
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

func fileBytes(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "none"
	}
	return string(b)
}

func fileStat(path string) string {
	st, err := os.Stat(path)
	if err != nil {
		return "none"
	}
	return fmt.Sprintf("%d|%d", st.Size(), st.ModTime().Unix()/2)
}

// menuInputs fingerprints every group the menu is built from.
func menuInputs(root string, c *Card) map[string]string {
	sw := filepath.Join(root, editsDir)
	var games []string
	for _, g := range c.Games {
		games = append(games, strings.Join([]string{g.Folder, g.Name, g.Product, g.Region, g.Disc, g.Date, g.Version, g.Type, fmt.Sprint(g.VGA), g.VGAState}, "|"))
	}
	look := []string{
		fileStat(logoPath(root)),
		fileStat(filepath.Join(sw, "BGM.THEME")), fileStat(filepath.Join(sw, "BGM.NONE")),
		dirListing(filepath.Join(sw, bgDir)),
	}
	for n := 1; n <= 3; n++ {
		look = append(look, fileStat(filepath.Join(sw, trackFile(n))))
	}
	look = append(look, fileStat(filepath.Join(sw, "BGM.ADP")))
	return map[string]string{
		"games":       fingerprint(games...),
		"edits":       fingerprint(fileBytes(filepath.Join(sw, "games.json"))),
		"art":         fingerprint(dirListing(filepath.Join(sw, "art"))),
		"collections": fingerprint(fileBytes(filepath.Join(sw, "collections.json"))),
		"look":        fingerprint(look...),
		"extras":      fingerprint(fileStat(filepath.Join(sw, "CODEBREAKER")), dirListing(filepath.Join(sw, "shots")), fileBytes(filepath.Join(sw, "vmucap.json")), fileStat(filepath.Join(sw, legacyININame))),
		"version":     fingerprint(version, swirlHash(swirlBinary)),
	}
}

// writeMenuStamp records what the menu was just built from. A failure here is not a failure of the build.
func writeMenuStamp(root string, log Logger) {
	c, err := ScanCard(root)
	if err != nil {
		return
	}
	st := buildStamp{Built: time.Now(), Version: version, Menu: swirlHash(swirlBinary), Sections: menuInputs(root, c)}
	b, _ := json.MarshalIndent(st, "", "  ")
	if err := os.WriteFile(menuStampPath(root), b, 0o644); err != nil {
		log("The menu stamp could not be written (%v); Changes waiting will say nothing until the next update", err)
	}
}

// PendingChanges compares the card with its stamp.
func PendingChanges(root string, c *Card) Pending {
	var st buildStamp
	b, err := os.ReadFile(menuStampPath(root))
	if err != nil || json.Unmarshal(b, &st) != nil || st.Sections == nil {
		return Pending{Sections: []string{}}
	}
	now := menuInputs(root, c)
	p := Pending{Known: true, Sections: []string{}, Built: st.Built}
	for _, k := range pendingKeys {
		if now[k] != st.Sections[k] {
			p.Sections = append(p.Sections, k)
		}
	}
	return p
}
