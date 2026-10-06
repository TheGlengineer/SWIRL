package main

// Batch edit (2.17): the same change applied to several games at once from the Games list. Only the fields
// the owner set in the window change; everything else about each game (its name, art, descriptions, the
// other info fields) stays as it is. The result is the same per game edit the Edit window writes, so the
// menu build and the Edit window see no difference.

import (
	"errors"
	"strings"
)

type BatchEdit struct {
	Root    string   `json:"root"`
	Folders []string `json:"folders"`
	Region  string   `json:"region,omitempty"`  // "" leave, else USA / EUR / JPN ...
	VGA     *bool    `json:"vga,omitempty"`     // nil leave
	Players *int     `json:"players,omitempty"` // nil leave
	Network *int     `json:"network,omitempty"` // nil leave
	Genres  uint16   `json:"genres,omitempty"`  // bits to add
	Acc     uint16   `json:"acc,omitempty"`     // bits to add
	// ClearGenres / ClearAcc replace the bits instead of adding to them
	ClearGenres bool `json:"clearGenres,omitempty"`
	ClearAcc    bool `json:"clearAcc,omitempty"`
}

func (b *BatchEdit) empty() bool {
	return b.Region == "" && b.VGA == nil && b.Players == nil && b.Network == nil && b.Genres == 0 && b.Acc == 0 && !b.ClearGenres && !b.ClearAcc
}

// currentMeta is the info a game has now: the owner's edit, else the menu disc's META.DAT record, else none.
func currentMeta(root string, m *menuReader, g *Game, e *GameEdit) (Meta, bool) {
	if e != nil && e.Meta != nil {
		return *e.Meta, true
	}
	if m != nil {
		if b, err := m.datChunk("META.DAT", g.Product); err == nil {
			return decodeMeta(b), true
		}
	}
	return Meta{}, false
}

// ApplyBatchEdit changes the picked games and says how many it touched.
func ApplyBatchEdit(req BatchEdit) (int, error) {
	if req.empty() {
		return 0, errors.New("nothing to change")
	}
	if len(req.Folders) == 0 {
		return 0, errors.New("no games picked")
	}
	c, err := ScanCard(req.Root)
	if err != nil {
		return 0, err
	}
	edits := loadEdits(req.Root)
	m, _ := openMenuDisc(req.Root)
	if m != nil {
		defer m.Close()
	}
	needMeta := req.Players != nil || req.Network != nil || req.Genres != 0 || req.Acc != 0 || req.ClearGenres || req.ClearAcc
	n := 0
	for _, folder := range req.Folders {
		g := findGame(c, folder)
		if g == nil || !folderRe.MatchString(folder) {
			continue
		}
		e := edits.Games[folder]
		if e == nil || e.Product != g.Product {
			e = &GameEdit{Product: g.Product, UserName: g.UserName}
		}
		if req.Region != "" {
			e.Region = strings.ToUpper(strings.TrimSpace(req.Region))
		}
		if req.VGA != nil {
			v := *req.VGA
			e.VGA = &v
		}
		if needMeta {
			meta, _ := currentMeta(req.Root, m, g, e)
			if req.Players != nil {
				meta.Players = clamp(*req.Players, 0, 255)
			}
			if req.Network != nil {
				meta.Network = clamp(*req.Network, 0, 3)
			}
			if req.ClearGenres {
				meta.Genre = 0
			}
			if req.ClearAcc {
				meta.Accessories = 0
			}
			meta.Genre |= req.Genres
			meta.Accessories |= req.Acc
			e.Meta = &meta
		}
		edits.Games[folder] = e
		n++
	}
	if n == 0 {
		return 0, errors.New("none of the picked games is on the card")
	}
	return n, edits.save()
}
