package main

// Smart collections (2.17): a collection can carry a rule beside its picked games. The rule is answered from
// the game info on the card (META.DAT after the online database and the owner's edits): any of the chosen
// genres, at least so many players, online play, any of the chosen accessories, a region. The games the
// rule finds join the picked ones every time the menu is built, so a game added next month lands in
// "Racing" by itself. SWIRL reads the result as COLLECT.TXT, the same file as before.

import (
	"os"
	"sort"
	"strings"
)

type ColRule struct {
	Genres      uint16   `json:"genres,omitempty"`      // any of these genre bits (db_item genre)
	MinPlayers  int      `json:"minPlayers,omitempty"`  // 0: any
	Online      bool     `json:"online,omitempty"`      // modem or broadband play
	Accessories uint16   `json:"accessories,omitempty"` // any of these accessory bits
	Regions     []string `json:"regions,omitempty"`     // "USA", "EUR", "JPN" ...
}

func (r *ColRule) empty() bool {
	return r == nil || (r.Genres == 0 && r.MinPlayers == 0 && !r.Online && r.Accessories == 0 && len(r.Regions) == 0)
}

func (r *ColRule) matches(g *Game, m *Meta, hasMeta bool) bool {
	if r.empty() {
		return false
	}
	if len(r.Regions) > 0 {
		ok := false
		for _, reg := range r.Regions {
			if strings.EqualFold(strings.TrimSpace(reg), g.Region) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	if r.Genres != 0 || r.MinPlayers != 0 || r.Online || r.Accessories != 0 {
		if !hasMeta {
			return false
		}
		if r.Genres != 0 && m.Genre&r.Genres == 0 {
			return false
		}
		if r.MinPlayers != 0 && m.Players < r.MinPlayers {
			return false
		}
		if r.Online && m.Network == 0 {
			return false
		}
		if r.Accessories != 0 && m.Accessories&r.Accessories == 0 {
			return false
		}
	}
	return true
}

// metaByProduct reads the game info the menu has: dat is META.DAT, the one being built (data folder) or the
// one on the current menu disc; the owner's edits win either way.
func metaByProduct(root string, c *Card, dat []byte) map[string]Meta {
	out := map[string]Meta{}
	if d, err := parseDat(dat); err == nil {
		for id, chunk := range d.Chunks {
			out[id] = decodeMeta(chunk)
		}
	}
	edits := loadEdits(root)
	for i := range c.Games {
		g := &c.Games[i]
		if e := edits.Games[g.Folder]; e != nil && e.Product == g.Product && e.Meta != nil {
			out[g.Product] = *e.Meta
		}
	}
	return out
}

// ruleProducts lists the games a rule finds on the card, by serial, in card order.
func ruleProducts(r *ColRule, c *Card, metas map[string]Meta) []string {
	var out []string
	seen := map[string]bool{}
	for i := range c.Games {
		g := &c.Games[i]
		if g.Product == "" || seen[g.Product] {
			continue
		}
		m, has := metas[g.Product]
		if r.matches(g, &m, has) {
			seen[g.Product] = true
			out = append(out, g.Product)
		}
	}
	return out
}

// resolveCollections fills every collection's games for the menu: the picked ones, then what its rule finds.
func resolveCollections(root string, c *Card, list []Collection, metaDat string) []Collection {
	dat, _ := os.ReadFile(metaDat)
	var metas map[string]Meta
	out := make([]Collection, 0, len(list))
	for _, col := range list {
		prods := append([]string(nil), col.Products...)
		if !col.Rule.empty() {
			if metas == nil {
				metas = metaByProduct(root, c, dat)
			}
			have := map[string]bool{}
			for _, p := range prods {
				have[p] = true
			}
			for _, p := range ruleProducts(col.Rule, c, metas) {
				if !have[p] {
					have[p] = true
					prods = append(prods, p)
				}
			}
		}
		out = append(out, Collection{Name: col.Name, Products: prods, Rule: col.Rule})
	}
	return out
}

// RulePreview answers the Collections window: which games a rule finds right now.
func RulePreview(root string, c *Card, r *ColRule) ([]string, error) {
	var dat []byte
	if m, err := openMenuDisc(root); err == nil {
		dat, _ = m.readFile("META.DAT")
		m.Close()
	}
	metas := metaByProduct(root, c, dat)
	prods := ruleProducts(r, c, metas)
	sort.Strings(prods)
	return prods, nil
}
