package main

// DISCDB.JSON is the Virtual Folder Bundle's card database: {"version":1,"items":{"NN":{name, serial,
// type, disc, vga, region, version, date, folder, altFolders, shrunk}}}, keyed by folder name. Its card
// manager trusts the entry for a folder without looking at the disc, so after SWIRL adds, removes or
// renumbers games the file must say what each folder holds now, or going back to the Bundle shows the
// wrong names and serials. Every menu rebuild brings it in step; a file SWIRL cannot read is moved to
// SWIRL_BACKUP with a note, and the Bundle rebuilds it from name.txt and serial.txt.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const discDBName = "DISCDB.JSON"

func discDBPath(root string) string { return findRootFile(root, discDBName) }

// findRootFile returns the path of name in root, matching case insensitively, or root/name when absent.
func findRootFile(root, name string) string {
	if p := findDataFile(root, name); p != "" {
		return p
	}
	return filepath.Join(root, name)
}

type discDBEntry map[string]any

func (e discDBEntry) str(k string) string {
	s, _ := e[k].(string)
	return s
}

// readDiscDB parses the file generically so keys SWIRL does not know survive a rewrite. An error means
// the file exists but cannot be kept in step.
func readDiscDB(path string) (items map[string]discDBEntry, top map[string]json.RawMessage, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, nil, fmt.Errorf("not valid JSON: %v", err)
	}
	var ver int
	if raw, ok := top["version"]; !ok || json.Unmarshal(raw, &ver) != nil || ver != 1 {
		return nil, nil, errors.New("its version is not 1")
	}
	raw, ok := top["items"]
	if !ok {
		return nil, nil, errors.New("it has no items")
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, nil, fmt.Errorf("its items cannot be read: %v", err)
	}
	if items == nil {
		items = map[string]discDBEntry{}
	}
	return items, top, nil
}

func writeDiscDB(path string, items map[string]discDBEntry, top map[string]json.RawMessage) error {
	out := map[string]any{}
	for k, v := range top {
		out[k] = v
	}
	out["version"] = 1
	out["items"] = items
	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := writeCardFile(tmp, b); err != nil {
		return err
	}
	if err := cardfs.Rename(tmp, path); err != nil {
		cardfs.Remove(tmp)
		return err
	}
	return cardfs.SyncDir(filepath.Dir(path))
}

// sameSerial says whether two serials name the same disc, whichever way they are written.
func sameSerial(a, b string) bool {
	return a != "" && b != "" && serialKey(a) == serialKey(b)
}

// discDBEntryFor makes the Bundle's entry for a game from what the scan knows.
func discDBEntryFor(g Game) discDBEntry {
	typ := "game"
	if g.Type != "" {
		typ = g.Type
	}
	return discDBEntry{"name": g.Name, "serial": g.Product, "type": typ, "disc": g.Disc, "vga": g.VGA, "region": g.Region,
		"version": g.Version, "date": g.Date, "folder": "", "altFolders": []string{}, "shrunk": false}
}

// reconcileDiscDB returns the items as they should be for the card's folders: entries follow their game
// when folders were renumbered, stale ones are replaced, gone ones dropped, new games get an entry. The
// second result lists what changed, one line per folder.
func reconcileDiscDB(items map[string]discDBEntry, c *Card) (map[string]discDBEntry, []string) {
	out := map[string]discDBEntry{}
	used := map[string]bool{}
	var changes []string
	var pending []Game
	for _, g := range c.Games {
		if e, ok := items[g.Folder]; ok && sameSerial(e.str("serial"), g.Product) {
			out[g.Folder], used[g.Folder] = e, true
			if g.Custom && e.str("name") != g.Name {
				e["name"] = g.Name
				changes = append(changes, fmt.Sprintf("folder %s: name is now %s", g.Folder, g.Name))
			}
			continue
		}
		pending = append(pending, g)
	}
	var oldKeys []string
	for k := range items {
		oldKeys = append(oldKeys, k)
	}
	sort.Slice(oldKeys, func(i, j int) bool { return naturalLess(oldKeys[i], oldKeys[j]) })
	for _, g := range pending {
		moved := ""
		for _, k := range oldKeys {
			if !used[k] && sameSerial(items[k].str("serial"), g.Product) && items[k].str("disc") == g.Disc {
				moved = k
				break
			}
		}
		if moved == "" {
			for _, k := range oldKeys {
				if !used[k] && sameSerial(items[k].str("serial"), g.Product) {
					moved = k
					break
				}
			}
		}
		if moved != "" {
			e := items[moved]
			used[moved] = true
			if g.Custom && e.str("name") != g.Name {
				e["name"] = g.Name
			}
			out[g.Folder] = e
			changes = append(changes, fmt.Sprintf("folder %s: %s, was folder %s", g.Folder, g.Name, moved))
			continue
		}
		out[g.Folder] = discDBEntryFor(g)
		if old, had := items[g.Folder]; had {
			changes = append(changes, fmt.Sprintf("folder %s: %s, the file said %s", g.Folder, g.Name, old.str("name")))
		} else {
			changes = append(changes, fmt.Sprintf("folder %s: %s, new", g.Folder, g.Name))
		}
	}
	for _, k := range oldKeys {
		if _, kept := out[k]; !kept && !used[k] {
			changes = append(changes, fmt.Sprintf("folder %s: %s, no longer on the card", k, items[k].str("name")))
		}
	}
	return out, changes
}

// syncDiscDB brings DISCDB.JSON in step with the game folders after a menu rebuild. A file that cannot
// be read is moved aside with a note rather than left saying the wrong thing.
func syncDiscDB(root string, c *Card, log Logger) error {
	path := discDBPath(root)
	if !fileExists(path) {
		return nil
	}
	items, top, err := readDiscDB(path)
	if err != nil {
		return setAsideDiscDB(root, path, err.Error(), log)
	}
	want, changes := reconcileDiscDB(items, c)
	if len(changes) == 0 {
		return nil
	}
	if err := writeDiscDB(path, want, top); err != nil {
		return fmt.Errorf("updating %s: %w", discDBName, err)
	}
	log("Updated %s, the Virtual Folder Bundle's database, for %d folders", discDBName, len(changes))
	for _, ch := range changes {
		log("  %s", ch)
	}
	return nil
}

func setAsideDiscDB(root, path, why string, log Logger) error {
	stamp := time.Now().Format("20060102_150405")
	dest := filepath.Join(root, backupDir, "DISCDB_"+stamp+".JSON")
	if err := cardfs.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	if err := cardfs.Rename(path, dest); err != nil {
		return fmt.Errorf("moving %s aside: %w", discDBName, err)
	}
	note := fmt.Sprintf("SWIRL Card Manager moved DISCDB.JSON here on %s because it could not keep it in step with the game folders: %s.\r\n"+
		"The file was left saying what each folder held before SWIRL changed the card, so the Virtual Folder Bundle's card manager would have shown the wrong games.\r\n"+
		"That manager rebuilds the database from the name.txt and serial.txt files in each game folder, which SWIRL keeps right.\r\n",
		time.Now().Format("2006-01-02 15:04"), why)
	writeCardFile(filepath.Join(root, backupDir, "DISCDB_"+stamp+".TXT"), []byte(note))
	log("%s could not be read (%s); moved to %s with a note. The Virtual Folder Bundle rebuilds it from name.txt and serial.txt", discDBName, why, filepath.Join(backupDir, filepath.Base(dest)))
	return nil
}

// discDBProblems lists, for the health check, the folders DISCDB.JSON describes wrongly.
func discDBProblems(root string, c *Card) []string {
	path := discDBPath(root)
	if !fileExists(path) {
		return nil
	}
	items, _, err := readDiscDB(path)
	if err != nil {
		return []string{fmt.Sprintf("%s (the Virtual Folder Bundle's database) cannot be read: %s. The next menu update moves it to %s with a note", discDBName, err, backupDir)}
	}
	var out []string
	for _, g := range c.Games {
		e, ok := items[g.Folder]
		switch {
		case !ok:
			out = append(out, fmt.Sprintf("%s has no entry for folder %s (%s)", discDBName, g.Folder, g.Name))
		case !sameSerial(e.str("serial"), g.Product):
			out = append(out, fmt.Sprintf("%s says folder %s is %s (%s) but it holds %s (%s)", discDBName, g.Folder, e.str("name"), e.str("serial"), g.Name, g.Product))
		}
	}
	for k, e := range items {
		if findGame(c, k) == nil && folderRe.MatchString(k) {
			out = append(out, fmt.Sprintf("%s lists %s in folder %s, which is not on the card", discDBName, e.str("name"), k))
		}
	}
	sort.Strings(out)
	if len(out) > 0 {
		out = append(out, "The next menu update (Update SWIRL) brings "+discDBName+" in step with the folders")
	}
	return out
}

// hasDiscDB says whether the card came from the Virtual Folder Bundle.
func hasDiscDB(root string) bool { return fileExists(discDBPath(root)) }
