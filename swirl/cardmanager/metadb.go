package main

// Descriptions in the menu's languages (2.17). SWIRL shows a game's description in the language chosen under
// System > Language; the text comes from three places, the first that has it wins:
//
//	1. what the owner typed for that language in the game's Edit window (SWIRL/games.json, "desc")
//	2. the translation pack built into Card Manager: the community metadb descriptions translated into
//	   German, French, Spanish, Italian and Portuguese (assets/metadb/<code>.json, keyed by serial; a game
//	   whose serial is not in the pack but whose English text is still matches by that text)
//	3. the English in META.DAT, which the menu falls back to on its own
//
// At Update SWIRL one file per language goes on the menu disc beside META.DAT: META_DE.DAT, META_FR.DAT,
// META_ES.DAT, META_IT.DAT, META_PT.DAT, in the same DAT layout with the same 384 byte record, the description
// in UTF-8 (the menu's fonts hold Latin-1). META.DAT itself is not changed, so stock openMenu and every other
// tool keep reading the card as before, and a menu older than 2.17 ignores the extra files.

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

//go:embed assets/metadb/*.json
var metadbFiles embed.FS

// descLangs are the languages SWIRL speaks beside English, in the order the Edit window lists them.
var descLangs = []string{"de", "fr", "es", "it", "pt"}

var descLangNames = map[string]string{"en": "English", "de": "Deutsch", "fr": "Français", "es": "Español", "it": "Italiano", "pt": "Português"}

func descFileName(lang string) string { return "META_" + strings.ToUpper(lang) + ".DAT" }

type descPackData struct {
	text   map[string]map[string]string // lang -> serial -> description
	bySrc  map[string]string            // normalised English text -> serial
	loaded bool
}

var (
	descPackOnce sync.Once
	descPackVal  descPackData
)

// descPack loads the embedded pack once. A damaged file leaves that language empty rather than failing the
// build: the card then carries only the owner's own text for it.
func descPack() *descPackData {
	descPackOnce.Do(func() {
		p := &descPackVal
		p.text = map[string]map[string]string{}
		p.bySrc = map[string]string{}
		var en map[string]string
		if b, err := metadbFiles.ReadFile("assets/metadb/en.json"); err == nil {
			json.Unmarshal(b, &en)
		}
		for serial, txt := range en {
			if k := descKey(txt); k != "" {
				if _, dup := p.bySrc[k]; !dup {
					p.bySrc[k] = serial
				}
			}
		}
		for _, lang := range descLangs {
			m := map[string]string{}
			if b, err := metadbFiles.ReadFile("assets/metadb/" + lang + ".json"); err == nil {
				json.Unmarshal(b, &m)
			}
			for serial, txt := range m {
				m[serial] = descFit(txt)
			}
			p.text[lang] = m
		}
		p.loaded = true
	})
	return &descPackVal
}

// descKey normalises an English description so the card's copy (ASCII, maybe re-spaced by an editor) still
// finds the pack entry it came from.
func descKey(s string) string {
	s = strings.ToLower(asciiOnly(s))
	return strings.Join(strings.Fields(s), " ")
}

// packDesc is the pack's text for a game in one language: by serial, else by its English description.
func (p *descPackData) packDesc(lang, serial, english string) string {
	m := p.text[lang]
	if m == nil {
		return ""
	}
	if serial != "" {
		if t := m[serial]; t != "" {
			return t
		}
	}
	if k := descKey(english); k != "" {
		if s := p.bySrc[k]; s != "" {
			return m[s]
		}
	}
	return ""
}

// descFit makes a description storable: one line, straight quotes, Latin-1 only (the menu's fonts), at most
// 375 bytes of UTF-8, never cut inside a character.
func descFit(s string) string {
	repl := strings.NewReplacer("‘", "'", "’", "'", "“", "\"", "”", "\"", "–", "-", "—", "-", "…", "...", "\r", "", "\n", " ", "\t", " ")
	s = repl.Replace(s)
	var b strings.Builder
	for _, r := range s {
		if r >= 32 && r <= 0xFF && r != 127 {
			b.WriteRune(r)
		}
	}
	s = strings.TrimSpace(b.String())
	const max = 375
	for len(s) > max {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

// encodeMetaLang is a META record for a language file: the English record's numbers with the translated text.
func encodeMetaLang(base []byte, desc string) []byte {
	b := make([]byte, metaSize)
	if len(base) >= 8 {
		copy(b[:8], base[:8])
	}
	copy(b[8:], descFit(desc))
	return b
}

// cardDescs collects, per language, the description each game on the card should show: the owner's text
// first, then the pack. english holds the META.DAT text per serial (after the owner's edits were applied).
func cardDescs(root string, c *Card, english map[string]string) map[string]map[string]string {
	edits := loadEdits(root)
	pack := descPack()
	out := map[string]map[string]string{}
	for _, lang := range descLangs {
		m := map[string]string{}
		for i := range c.Games {
			g := &c.Games[i]
			if g.Product == "" {
				continue
			}
			if _, done := m[g.Product]; done {
				continue // two discs of one game share the serial
			}
			txt := ""
			if e := edits.Games[g.Folder]; e != nil && e.Product == g.Product && e.Desc != nil {
				txt = descFit(e.Desc[lang])
			}
			if txt == "" {
				txt = pack.packDesc(lang, g.Product, english[g.Product])
			}
			if txt != "" {
				m[g.Product] = txt
			}
		}
		out[lang] = m
	}
	return out
}

// addLanguageMeta writes the META_xx.DAT files into the menu data folder after META.DAT is final (the online
// database merged and the owner's edits applied), and removes a language file the card no longer needs.
func addLanguageMeta(root string, c *Card, data string, log Logger) error {
	english := map[string]string{}
	var base map[string][]byte
	if d, err := readDat(filepath.Join(data, "META.DAT")); err == nil {
		base = d.Chunks
		for id, chunk := range d.Chunks {
			english[id] = decodeMeta(chunk).Description
		}
	}
	descs := cardDescs(root, c, english)
	var counts []string
	for _, lang := range descLangs {
		name := descFileName(lang)
		path := filepath.Join(data, name)
		m := descs[lang]
		if len(m) == 0 {
			if fileExists(path) {
				os.Remove(path)
			}
			continue
		}
		d := newDat(metaSize)
		ids := make([]string, 0, len(m))
		for id := range m {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			d.Set(id, encodeMetaLang(base[id], m[id]))
		}
		if err := d.Write(path); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
		counts = append(counts, fmt.Sprintf("%s %d", descLangNames[lang], len(m)))
	}
	if len(counts) > 0 {
		log("Descriptions in the menu's languages: %s", strings.Join(counts, ", "))
	}
	return nil
}

// GameDescs is what the Edit window shows under Description: the owner's own text per language and the
// pack's text, so the window can tell which is which.
type GameDescs struct {
	Own  map[string]string `json:"own"`  // lang -> the owner's text ("" when none)
	Pack map[string]string `json:"pack"` // lang -> the pack's text ("" when none)
}

func gameDescs(root string, g *Game, english string) GameDescs {
	out := GameDescs{Own: map[string]string{}, Pack: map[string]string{}}
	edits := loadEdits(root)
	pack := descPack()
	for _, lang := range descLangs {
		if e := edits.Games[g.Folder]; e != nil && e.Product == g.Product && e.Desc != nil {
			out.Own[lang] = e.Desc[lang]
		}
		out.Pack[lang] = pack.packDesc(lang, g.Product, english)
	}
	return out
}

// cleanDescs keeps only the languages with text, fitted; nil when there is nothing to keep.
func cleanDescs(in map[string]string) map[string]string {
	var out map[string]string
	for _, lang := range descLangs {
		if t := descFit(in[lang]); t != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[lang] = t
		}
	}
	return out
}
