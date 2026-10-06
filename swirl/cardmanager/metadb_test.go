package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// The pack: every language covers every English entry, and every text fits a META record as the menu reads it.
func TestDescPack(t *testing.T) {
	p := descPack()
	if len(p.bySrc) < 400 {
		t.Fatalf("English index has %d entries", len(p.bySrc))
	}
	for _, lang := range descLangs {
		m := p.text[lang]
		if len(m) != len(p.text["de"]) || len(m) < len(p.bySrc) { // serials share a text, so the index is smaller
			t.Errorf("%s: %d entries, English index %d", lang, len(m), len(p.bySrc))
		}
		for id, txt := range m {
			if txt == "" || len(txt) > 375 || !utf8.ValidString(txt) {
				t.Errorf("%s %s: %d bytes", lang, id, len(txt))
			}
			for _, r := range txt {
				if r > 0xFF || r < 32 {
					t.Errorf("%s %s: character %U outside the menu's fonts", lang, id, r)
				}
			}
			if strings.ContainsAny(txt, "–—\n") {
				t.Errorf("%s %s: dash or newline", lang, id)
			}
		}
	}
	// a known title, by serial and by its English text (another region's serial with the same blurb)
	if d := p.packDesc("de", "T36805N", ""); !strings.Contains(d, "Guts") {
		t.Fatal("by serial:", d)
	}
	en := "An outcast warrior enters a land plagued by an evil curse. Guts is unlike ordinary men for he carries the Dragon Slayer, a mighty blade of retribution whose fury knows no equal. His enemies will know true fear once they encounter his Berserk rage!"
	if d := p.packDesc("fr", "T99999X", "  "+strings.ToUpper(en[:1])+en[1:]+" "); !strings.Contains(d, "Guts") {
		t.Fatal("by text:", d)
	}
	if d := p.packDesc("it", "T99999X", "Nothing like this was ever written."); d != "" {
		t.Fatal("unknown text matched:", d)
	}
}

func TestDescFit(t *testing.T) {
	if got := descFit("  a “quoted” line\r\nwith — dash and 中  "); got != "a \"quoted\" line with - dash and" {
		t.Fatalf("%q", got)
	}
	long := strings.Repeat("é", 200) // 400 bytes of two byte characters
	got := descFit(long)
	if len(got) != 374 || !utf8.ValidString(got) || utf8.RuneCountInString(got) != 187 {
		t.Fatalf("%d bytes, %d runes", len(got), utf8.RuneCountInString(got))
	}
	rec := encodeMetaLang([]byte{2, 10, 0, 1, 3, 0, 5, 0}, got)
	if len(rec) != metaSize || rec[0] != 2 || rec[1] != 10 || rec[3] != 1 || decodeMetaUTF8(rec) != got {
		t.Fatal("record")
	}
}

func decodeMetaUTF8(b []byte) string { return strings.TrimRight(string(b[8:]), "\x00") }

// At Update SWIRL: the owner's text wins over the pack, the pack covers games by serial or English text,
// games with nothing stay out, a language file nobody needs is removed, and the files are DAT files of
// META records.
func TestLanguageMetaFiles(t *testing.T) {
	root := t.TempDir()
	data := t.TempDir()
	os.MkdirAll(filepath.Join(root, editsDir), 0o755)
	c := &Card{Games: []Game{
		{Folder: "02", Product: "T36805N", Name: "Berserk"},
		{Folder: "03", Product: "HOMEBREW1", Name: "My game"},
		{Folder: "04", Product: "T99999X", Name: "Berserk PAL"},
		{Folder: "05", Product: "T36805N", Name: "Berserk disc 2"},
	}}
	en := "An outcast warrior enters a land plagued by an evil curse. Guts is unlike ordinary men for he carries the Dragon Slayer, a mighty blade of retribution whose fury knows no equal. His enemies will know true fear once they encounter his Berserk rage!"
	d := newDat(metaSize)
	d.Set("T36805N", encodeMeta(Meta{Players: 1, Description: en}))
	d.Set("HOMEBREW1", encodeMeta(Meta{Players: 2, Description: "A homebrew game."}))
	d.Set("T99999X", encodeMeta(Meta{Players: 1, Description: en}))
	if err := d.Write(filepath.Join(data, "META.DAT")); err != nil {
		t.Fatal(err)
	}
	edits := loadEdits(root)
	edits.Games["02"] = &GameEdit{Product: "T36805N", Desc: map[string]string{"de": "Meine eigene Beschreibung für Guts."}}
	edits.Games["03"] = &GameEdit{Product: "HOMEBREW1", Desc: map[string]string{"pt": "Um jogo caseiro."}}
	edits.save()
	os.WriteFile(filepath.Join(data, "META_IT.DAT"), []byte("stale"), 0o644)
	// no Italian for anyone? the pack has Italian for Berserk, so it stays; make a stale file for a language
	// the pack lacks by hiding it
	var logs []string
	if err := addLanguageMeta(root, c, data, func(f string, a ...any) { logs = append(logs, f) }); err != nil {
		t.Fatal(err)
	}
	de, err := readDat(filepath.Join(data, "META_DE.DAT"))
	if err != nil {
		t.Fatal(err)
	}
	if de.ChunkSize != metaSize || len(de.Order) != 2 {
		t.Fatalf("German: chunk %d, %v", de.ChunkSize, de.Order)
	}
	if got := decodeMetaUTF8(de.Chunks["T36805N"]); got != "Meine eigene Beschreibung für Guts." {
		t.Fatal("own text lost:", got)
	}
	if got := decodeMetaUTF8(de.Chunks["T99999X"]); !strings.Contains(got, "Guts") || de.Chunks["T99999X"][0] != 1 {
		t.Fatal("pack by text:", got)
	}
	if _, has := de.Chunks["HOMEBREW1"]; has {
		t.Fatal("homebrew got a German text from nowhere")
	}
	pt, err := readDat(filepath.Join(data, "META_PT.DAT"))
	if err != nil || len(pt.Order) != 3 || decodeMetaUTF8(pt.Chunks["HOMEBREW1"]) != "Um jogo caseiro." {
		t.Fatal("Portuguese", err)
	}
	it, err := readDat(filepath.Join(data, "META_IT.DAT"))
	if err != nil || len(it.Order) != 2 {
		t.Fatal("Italian stale file not replaced", err)
	}
	// a card without any of those games gets no files, and the old ones go
	c2 := &Card{Games: []Game{{Folder: "02", Product: "HOMEBREW1"}}}
	d2 := newDat(metaSize)
	d2.Set("HOMEBREW1", encodeMeta(Meta{Description: "A homebrew game."}))
	d2.Write(filepath.Join(data, "META.DAT"))
	if err := addLanguageMeta(t.TempDir(), c2, data, func(string, ...any) {}); err != nil {
		t.Fatal(err)
	}
	for _, lang := range descLangs {
		if fileExists(filepath.Join(data, descFileName(lang))) {
			t.Fatal("stale", lang)
		}
	}
	// the Edit window sees own and pack text apart
	g := &c.Games[0]
	ds := gameDescs(root, g, en)
	if ds.Own["de"] == "" || ds.Pack["de"] == "" || ds.Own["fr"] != "" || ds.Pack["fr"] == "" {
		t.Fatalf("%+v", ds)
	}
	if cleanDescs(map[string]string{"de": "  ", "fr": "Bonjour", "xx": "no"}) == nil || cleanDescs(map[string]string{"de": ""}) != nil {
		t.Fatal("cleanDescs")
	}
}
