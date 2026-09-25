package main

// Multi disc games: the discs of one game are recognised as a set (same serial, or same name, with the
// same disc count), shown together in the Games list and kept in neighbouring folders, disc 1 first.

import (
	"fmt"
	"sort"
	"strings"
)

type DiscSet struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Discs   int      `json:"discs"`   // how many discs the game has
	Folders []string `json:"folders"` // on the card, in disc order
	Missing []int    `json:"missing,omitempty"`
	Apart   bool     `json:"apart"` // not in neighbouring folders in disc order
}

func findDiscSets(games []Game) []DiscSet {
	parent := make([]int, len(games))
	for i := range parent {
		parent[i] = i
	}
	var find func(int) int
	find = func(i int) int {
		for parent[i] != i {
			parent[i] = parent[parent[i]]
			i = parent[i]
		}
		return i
	}
	byKey := map[string]int{}
	for i, g := range games {
		if g.DiscOf < 2 {
			continue
		}
		keys := []string{"n|" + strings.ToLower(strings.TrimSpace(g.Name)) + "|" + fmt.Sprint(g.DiscOf)}
		if k := serialKey(g.Product); k != "" {
			keys = append(keys, "p|"+k+"|"+fmt.Sprint(g.DiscOf))
		}
		for _, k := range keys {
			if j, ok := byKey[k]; ok {
				parent[find(i)] = find(j)
			} else {
				byKey[k] = i
			}
		}
	}
	groups := map[int][]int{}
	var roots []int
	for i, g := range games {
		if g.DiscOf < 2 {
			continue
		}
		r := find(i)
		if _, ok := groups[r]; !ok {
			roots = append(roots, r)
		}
		groups[r] = append(groups[r], i)
	}
	var out []DiscSet
	for _, r := range roots {
		idx := groups[r]
		sort.SliceStable(idx, func(a, b int) bool { return games[idx[a]].DiscNo < games[idx[b]].DiscNo })
		first := games[idx[0]]
		ds := DiscSet{ID: "set" + first.Folder, Name: first.Name, Discs: first.DiscOf}
		have := map[int]bool{}
		for _, i := range idx {
			ds.Folders = append(ds.Folders, games[i].Folder)
			have[games[i].DiscNo] = true
			games[i].Set = ds.ID
		}
		for d := 1; d <= ds.Discs; d++ {
			if !have[d] {
				ds.Missing = append(ds.Missing, d)
			}
		}
		// apart: the discs should sit in consecutive games, in disc order
		for k := 1; k < len(idx); k++ {
			if idx[k] != idx[k-1]+1 {
				ds.Apart = true
			}
		}
		out = append(out, ds)
	}
	return out
}

// arrangedOrder returns the folder order with the discs of every set moved next to each other, in disc
// order, where the set's first disc sits. changed lists the sets that moved.
func arrangedOrder(c *Card) (order []string, changed []string) {
	inSet := map[string]*DiscSet{}
	for i := range c.Sets {
		for _, f := range c.Sets[i].Folders {
			inSet[f] = &c.Sets[i]
		}
	}
	placed := map[string]bool{}
	for _, g := range c.Games {
		if placed[g.Folder] {
			continue
		}
		if s := inSet[g.Folder]; s != nil {
			for _, f := range s.Folders {
				if !placed[f] {
					order = append(order, f)
					placed[f] = true
				}
			}
			if s.Apart {
				changed = append(changed, s.Name)
			}
			continue
		}
		order = append(order, g.Folder)
		placed[g.Folder] = true
	}
	return order, dedupStrings(changed)
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// keepDiscsTogether renumbers folders so every set's discs are neighbours. It returns how many folders moved.
func keepDiscsTogether(root string, log Logger) (int, error) {
	c, err := ScanCard(root)
	if err != nil {
		return 0, err
	}
	order, changed := arrangedOrder(c)
	if len(changed) == 0 {
		return 0, nil
	}
	n, err := reorderFolders(root, order, log)
	if err == nil {
		for _, name := range changed {
			log("Put the discs of %s next to each other", name)
		}
	}
	return n, err
}

func StartArrangeDiscs(root string) error {
	return runJob("Putting discs together", "Done. Put the card back in your GDEMU.", func() error {
		n, err := keepDiscsTogether(root, jobLog)
		if err != nil {
			return err
		}
		if n == 0 {
			jobLog("The discs of every game are already next to each other.")
			return nil
		}
		jobUpdate(func(j *jobState) { j.Stage, j.Pct = "Rebuilding the menu", 0.5 })
		return installSwirl(root, "", true, jobLog)
	})
}
