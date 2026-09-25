package main

// Proper game names. Discs only carry a short upper case title ("18WHEELER"), and downloads are named
// after whoever packed them, so new games get their real title from the Redump Dreamcast list (built into
// the app by tools/gen_titles.py), or failing that a tidied up version of the file or disc name.

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

//go:embed assets/titles.tsv.gz
var titlesGz []byte

type titleEntry struct {
	Region, Title string
	Disc, Discs   int
}

var (
	titlesOnce sync.Once
	titleIndex map[string][]titleEntry
)

func serialKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	k := b.String()
	if len(k) > 2 && strings.HasPrefix(k, "MK") && k[2] >= '0' && k[2] <= '9' {
		k = k[2:]
	}
	return k
}

func loadTitles() {
	titleIndex = map[string][]titleEntry{}
	zr, err := gzip.NewReader(bytes.NewReader(titlesGz))
	if err != nil {
		return
	}
	sc := bufio.NewScanner(zr)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		if len(f) != 5 {
			continue
		}
		d, _ := strconv.Atoi(f[3])
		n, _ := strconv.Atoi(f[4])
		titleIndex[f[0]] = append(titleIndex[f[0]], titleEntry{Region: f[1], Title: f[2], Disc: d, Discs: n})
	}
}

// lookupTitle finds the Redump entry for a serial and disc number (0 when unknown). USA entries win when
// a serial is shared between regions.
func lookupTitle(product string, disc int) (titleEntry, bool) {
	titlesOnce.Do(loadTitles)
	list := titleIndex[serialKey(product)]
	if len(list) == 0 {
		return titleEntry{}, false
	}
	score := func(e titleEntry) int {
		s := 0
		if disc > 0 && e.Disc == disc {
			s += 10
		}
		switch e.Region {
		case "USA":
			s += 3
		case "Europe":
			s += 2
		case "Japan":
			s += 1
		}
		return s
	}
	best := list[0]
	for _, e := range list[1:] {
		if score(e) > score(best) {
			best = e
		}
	}
	return best, true
}

var (
	tagRe      = regexp.MustCompile(`\s*[\(\[][^\)\]]*[\)\]]`)
	discWordRe = regexp.MustCompile(`(?i)[\s_\-\.]*(disc|disk|cd)[\s_\-\.]*\d+(\s*of\s*\d+)?\b`)
	spaceRe    = regexp.MustCompile(`\s+`)
	romanRe    = regexp.MustCompile(`^(?i)(ii|iii|iv|vi|vii|viii|ix|x|xi|xii|xiii)$`)
)

// cleanLabel tidies a file or folder name: "Crazy_Taxi_(USA)_[!]" becomes "Crazy Taxi".
func cleanLabel(s string) string {
	s = tagRe.ReplaceAllString(s, "")
	s = strings.NewReplacer("_", " ", "+", " ").Replace(s)
	if !strings.Contains(s, " ") {
		s = strings.ReplaceAll(s, ".", " ")
	}
	s = discWordRe.ReplaceAllString(s, "")
	s = spaceRe.ReplaceAllString(s, " ")
	s = strings.Trim(s, " -.,")
	if m := regexp.MustCompile(`^(.*), (The|A|An)$`).FindStringSubmatch(s); m != nil {
		s = m[2] + " " + m[1]
	}
	if s == strings.ToUpper(s) || s == strings.ToLower(s) {
		s = titleCase(s)
	}
	return s
}

var smallWords = map[string]bool{"a": true, "an": true, "and": true, "at": true, "by": true, "for": true, "in": true, "of": true, "on": true, "or": true, "the": true, "to": true, "vs": true, "vs.": true}

// titleCase turns "SONIC ADVENTURE II" into "Sonic Adventure II".
func titleCase(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		switch {
		case romanRe.MatchString(w):
			words[i] = strings.ToUpper(w)
		case i > 0 && smallWords[w]:
		case w == "usa" || w == "dc" || w == "nfl" || w == "nba" || w == "nhl" || w == "wwf" || w == "ufc" || w == "4x4":
			words[i] = strings.ToUpper(w)
		default:
			r := []rune(w)
			r[0] = unicode.ToUpper(r[0])
			words[i] = string(r)
		}
	}
	return strings.Join(words, " ")
}

// meaningful is false for labels that say nothing about the game ("disc", "track01", "02", "GDI").
func meaningful(s string) bool {
	if genericName(s) || len(s) < 2 {
		return false
	}
	low := strings.ToLower(s)
	if folderRe.MatchString(s) || strings.HasPrefix(low, "track") || strings.HasPrefix(low, "folder ") {
		return false
	}
	letters := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			letters++
		}
	}
	return letters >= 2
}

// discNumbers reads "1/2" style disc fields.
func discNumbers(disc string) (int, int) {
	var d, n int
	if _, err := fmt.Sscanf(disc, "%d/%d", &d, &n); err != nil || d < 1 || n < 1 {
		return 1, 1
	}
	return d, n
}

// properName picks the name a newly added game should get. label is the file, folder or archive name
// it came from.
func properName(ip *ipInfo, label string) string {
	if ip != nil {
		d, _ := discNumbers(ip.Disc)
		if e, ok := lookupTitle(ip.Product, d); ok {
			return e.Title
		}
	}
	if l := cleanLabel(label); meaningful(l) {
		return l
	}
	if ip != nil && meaningful(ip.Name) {
		return titleCase(ip.Name)
	}
	return strings.TrimSpace(label)
}

// suggestedName is the proper name for a game already on the card, or "" when there is nothing better.
func suggestedName(g *Game) string {
	d, _ := discNumbers(g.Disc)
	if e, ok := lookupTitle(g.Product, d); ok {
		return e.Title
	}
	if !g.Custom && meaningful(g.Name) && g.Name == strings.ToUpper(g.Name) {
		return titleCase(g.Name)
	}
	return ""
}
