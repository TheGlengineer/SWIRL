package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// readLangDat parses a LANG.DAT the way the menu does (ui/swirl/sw_lang.c) and returns, per language
// code, the translated strings by key index.
func readLangDat(t *testing.T, dat []byte, keys []string) map[byte][]string {
	t.Helper()
	if len(dat) < 16 || string(dat[:4]) != "SWL1" {
		t.Fatalf("bad header")
	}
	le := binary.LittleEndian
	if le.Uint32(dat[4:]) != langTableHash(keys) {
		t.Fatalf("table hash differs")
	}
	count, n := int(le.Uint32(dat[8:])), int(le.Uint32(dat[12:]))
	if count != len(keys) {
		t.Fatalf("count %d, keys %d", count, len(keys))
	}
	out := map[byte][]string{}
	for i := 0; i < n; i++ {
		e := dat[16+16*i:]
		code := e[0]
		off, size := int(le.Uint32(e[4:])), int(le.Uint32(e[8:]))
		if off < 16+16*n || off+size > len(dat) || size < 4*count {
			t.Fatalf("language %d: bad entry off %d size %d", code, off, size)
		}
		block := dat[off : off+size]
		strs := make([]string, count)
		for k := 0; k < count; k++ {
			so := int(le.Uint32(block[4*k:]))
			if so == 0 {
				continue
			}
			if so < 4*count || so >= size {
				t.Fatalf("language %d key %s: bad offset %d", code, keys[k], so)
			}
			end := so
			for end < size && block[end] != 0 {
				end++
			}
			if end == size {
				t.Fatalf("language %d key %s: string runs off the block", code, keys[k])
			}
			strs[k] = string(block[so:end])
		}
		out[code] = strs
	}
	return out
}

func TestLangDat(t *testing.T) {
	dat, problems, err := buildLangDat()
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("translations left out: %v", problems)
	}
	keys, _ := langKeys()
	langs := readLangDat(t, dat, keys)
	if len(langs) != 5 {
		t.Fatalf("%d languages", len(langs))
	}
	idx := map[string]int{}
	for i, k := range keys {
		idx[k] = i
	}
	if langs[2][idx["S_LANG_NAME"]] != "Deutsch" || langs[3][idx["S_TAB_HOME"]] != "Accueil" ||
		langs[6][idx["S_LANG_NAME"]] != "Português" {
		t.Fatalf("strings: de %q fr %q pt %q", langs[2][idx["S_LANG_NAME"]], langs[3][idx["S_TAB_HOME"]],
			langs[6][idx["S_LANG_NAME"]])
	}
	// every language translates the whole table: a missing string shows English on the console
	en, _ := langLoad("en")
	for code, strs := range langs {
		for i, k := range keys {
			if strs[i] == "" {
				t.Errorf("language %d: %s not translated", code, k)
			}
			if _, ok := en[k]; !ok {
				t.Errorf("%s not in en.json", k)
			}
		}
	}
}

func TestLangSpecCheck(t *testing.T) {
	if !langSpecMatch("Jugado %u veces. Última partida %s.", "Played %u times. Last played %s.") {
		t.Fatal("same specifiers should match")
	}
	if langSpecMatch("%s de %d", "%d of %d") || langSpecMatch("Slot %u", "Slot %02u") {
		t.Fatal("different specifiers should not match")
	}
	if langUnsupported("Español") != "" || langUnsupported("€ 5") != "€" {
		t.Fatal("Latin-1 check")
	}
}

func TestLangAssetsMatchRepo(t *testing.T) {
	// assets/lang is a copy of swirl/lang (lang_table.py export writes keys.txt and en.json there)
	for _, name := range []string{"keys.txt", "en.json", "de.json", "fr.json", "es.json", "it.json", "pt.json"} {
		want, err := os.ReadFile(filepath.Join("..", "lang", name))
		if err != nil {
			t.Skip("swirl/lang not next to this checkout")
		}
		got, err := langFiles.ReadFile("assets/lang/" + name)
		if err != nil || string(got) != string(want) {
			t.Fatalf("assets/lang/%s differs from swirl/lang/%s: copy it over", name, name)
		}
	}
}

// Update SWIRL puts LANG.DAT on the menu disc, and it is the file buildLangDat makes.
func TestLangDatOnMenuDisc(t *testing.T) {
	root := txnCard(t, "openMenu", 2)
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	m, err := openMenuDisc(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	f, ok := m.files["LANG.DAT"]
	if !ok {
		t.Fatal("no LANG.DAT on the menu disc")
	}
	b, err := m.d.readSectors(f.LBA, (f.Size+sectorSize-1)/sectorSize)
	if err != nil {
		t.Fatal(err)
	}
	want, _, _ := buildLangDat()
	if string(b[:f.Size]) != string(want) {
		t.Fatalf("LANG.DAT on the disc differs (%d bytes, want %d)", f.Size, len(want))
	}
}

// The window's language files (web/lang) parse and cover every key the window can show (keys.json, written
// by swirl/tools/cm_lang.py export). A missing entry would show English for that string.
func TestWindowLanguages(t *testing.T) {
	var keys []string
	b, err := webFS.ReadFile("web/lang/keys.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &keys); err != nil {
		t.Fatal(err)
	}
	if len(keys) < 700 {
		t.Fatalf("only %d keys", len(keys))
	}
	for _, code := range []string{"de", "fr", "es", "it", "pt"} {
		b, err := webFS.ReadFile("web/lang/" + code + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]string
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("%s: %v", code, err)
		}
		for _, k := range keys {
			if m[k] == "" {
				t.Errorf("%s: %q not translated", code, k)
			}
		}
	}
}
