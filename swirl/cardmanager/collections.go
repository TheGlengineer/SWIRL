package main

// Custom collections ("Couch co-op", "Glen's picks"): kept on the card in SWIRL/collections.json by
// serial, written to the menu disc as COLLECT.TXT, and shown in SWIRL's Collections tab.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Collection struct {
	Name     string   `json:"name"`
	Products []string `json:"products"`
}

func collectionsPath(root string) string { return filepath.Join(root, editsDir, "collections.json") }

func LoadCollections(root string) []Collection {
	var out []Collection
	if b, err := os.ReadFile(collectionsPath(root)); err == nil {
		json.Unmarshal(b, &out)
	}
	return out
}

func SaveCollections(root string, list []Collection) error {
	seen := map[string]bool{}
	var clean []Collection
	for _, c := range list {
		name := strings.TrimSpace(strings.NewReplacer("[", "(", "]", ")", "\r", " ", "\n", " ").Replace(c.Name))
		if name == "" {
			return errors.New("every collection needs a name")
		}
		if len(name) > 30 {
			name = name[:30]
		}
		if seen[strings.ToLower(name)] {
			return fmt.Errorf("there are two collections called %s", name)
		}
		seen[strings.ToLower(name)] = true
		var prods []string
		dup := map[string]bool{}
		for _, p := range c.Products {
			p = strings.TrimSpace(p)
			if p != "" && !dup[p] {
				dup[p] = true
				prods = append(prods, p)
			}
		}
		clean = append(clean, Collection{Name: name, Products: prods})
	}
	if len(clean) > 24 {
		return errors.New("SWIRL shows up to 24 collections of your own")
	}
	os.MkdirAll(filepath.Join(root, editsDir), 0o755)
	b, _ := json.MarshalIndent(clean, "", "  ")
	return os.WriteFile(collectionsPath(root), b, 0o644)
}

// collectText is the COLLECT.TXT that SWIRL reads.
func collectText(list []Collection) string {
	var b strings.Builder
	b.WriteString("# SWIRL collections, made in SWIRL Card Manager\r\n")
	for _, c := range list {
		fmt.Fprintf(&b, "[%s]\r\n", c.Name)
		for _, p := range c.Products {
			b.WriteString(p + "\r\n")
		}
	}
	return b.String()
}
