package main

// What the Virtual Folder Bundle keeps per game besides name.txt and serial.txt: type.txt (game, other,
// psx), disc.txt (a disc number override, "2/3"), folder.txt and folder_alt1..5.txt (the virtual folders
// the game is filed under). A card the Bundle migrated to database mode may have none of these files, so
// its DISCDB.JSON entry is the fallback. Virtual folders become SWIRL collections the first time SWIRL
// is installed; the type and disc go into the menu's list.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// vfbExtras is what the Bundle says about one game folder.
type vfbExtras struct {
	Type    string   // "" (game), "other", "psx"
	Disc    string   // "" or "N/M"
	Folders []string // virtual folders, main one first
}

var discRe = regexp.MustCompile(`^\d+/\d+$`)

func readVFBExtras(dir string, db discDBEntry) vfbExtras {
	var x vfbExtras
	switch t := strings.ToLower(readText(filepath.Join(dir, "type.txt"))); t {
	case "other", "psx":
		x.Type = t
	case "":
		if t := strings.ToLower(db.str("type")); t == "other" || t == "psx" {
			x.Type = t
		}
	}
	if d := readText(filepath.Join(dir, "disc.txt")); discRe.MatchString(d) {
		x.Disc = d
	} else if d := db.str("disc"); d != "" && discRe.MatchString(d) && d != "1/1" {
		x.Disc = d
	}
	names := []string{readText(filepath.Join(dir, "folder.txt"))}
	for i := 1; i <= 5; i++ {
		names = append(names, readText(filepath.Join(dir, fmt.Sprintf("folder_alt%d.txt", i))))
	}
	if strings.Join(names, "") == "" && db != nil {
		names = []string{db.str("folder")}
		if alts, ok := db["altFolders"].([]any); ok {
			for _, a := range alts {
				if s, ok := a.(string); ok {
					names = append(names, s)
				}
			}
		}
	}
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n != "" && !seen[strings.ToLower(n)] {
			seen[strings.ToLower(n)] = true
			x.Folders = append(x.Folders, n)
		}
	}
	return x
}

// readVFBDatabase returns the Bundle's entries when the card has a readable DISCDB.JSON.
func readVFBDatabase(root string) map[string]discDBEntry {
	if !hasDiscDB(root) {
		return nil
	}
	items, _, err := readDiscDB(discDBPath(root))
	if err != nil {
		return nil
	}
	return items
}

// vfbFoldersMarker says the virtual folders were already turned into collections once.
func vfbFoldersMarker(root string) string {
	return filepath.Join(root, editsDir, "vfb-folders-imported.txt")
}

// importVFBFolders turns the Bundle's virtual folders into SWIRL collections on the first install. It
// runs once; collections the owner makes or removes afterwards are theirs.
func importVFBFolders(root string, c *Card, log Logger) error {
	if fileExists(vfbFoldersMarker(root)) || len(LoadCollections(root)) > 0 {
		return nil
	}
	db := readVFBDatabase(root)
	byName := map[string]*Collection{}
	var order []string
	for _, g := range c.Games {
		if g.Product == "" {
			continue
		}
		for _, f := range readVFBExtras(filepath.Join(root, g.Folder), db[g.Folder]).Folders {
			k := strings.ToLower(f)
			col := byName[k]
			if col == nil {
				col = &Collection{Name: f}
				byName[k] = col
				order = append(order, k)
			}
			col.Products = append(col.Products, g.Product)
		}
	}
	if len(order) == 0 {
		return nil
	}
	var cols []Collection
	for _, k := range order {
		cols = append(cols, *byName[k])
	}
	if len(cols) > 24 {
		sort.SliceStable(cols, func(i, j int) bool { return len(cols[i].Products) > len(cols[j].Products) })
		log("The card has %d virtual folders; SWIRL keeps 24 collections, so the %d smallest were left out", len(cols), len(cols)-24)
		cols = cols[:24]
	}
	if err := SaveCollections(root, cols); err != nil {
		return fmt.Errorf("turning the virtual folders into collections: %w", err)
	}
	os.MkdirAll(filepath.Dir(vfbFoldersMarker(root)), 0o755)
	os.WriteFile(vfbFoldersMarker(root), []byte("The Virtual Folder Bundle's folders were made into SWIRL collections once. Delete this file to do it again.\r\n"), 0o644)
	var names []string
	for _, col := range cols {
		names = append(names, col.Name)
	}
	log("Made %d collections from the card's virtual folders: %s", len(cols), strings.Join(names, ", "))
	return nil
}
