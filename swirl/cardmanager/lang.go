package main

// The menu's translations: LANG.DAT on the menu disc, compiled from assets/lang/<code>.json (copies of
// swirl/lang, a test keeps them equal) in the key order of assets/lang/keys.txt (exported from
// ui/swirl/sw_lang.h by swirl/tools/lang_table.py). The menu reads the file as ui/swirl/sw_lang.c
// documents:
//
//	"SWL1", u32 table hash (FNV-1a of the keys joined by newlines), u32 strings per language, u32 languages,
//	per language 16 bytes: u8 code, 3 x u8 0, u32 offset from the file start, u32 size, u32 0,
//	per language block: u32 offset[strings] from the block start (0: not translated), then the UTF-8
//	strings, NUL terminated.
//
// A translation whose printf specifiers differ from the English is left out of the file (the menu shows
// the English for that string) and the build log says so, so a bad edit never reaches the console.

import (
	"bytes"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

//go:embed assets/lang/*.json assets/lang/keys.txt
var langFiles embed.FS

const langOnDisc = "LANG.DAT"

// langCodes maps a file's language code to the menu's SW_LANG_* value (sw_lang.h). English is built into
// the menu and never written.
var langCodes = map[string]byte{"de": 2, "fr": 3, "es": 4, "it": 5, "pt": 6}

var langSpec = regexp.MustCompile(`%[0-9]*[a-z]`)

// langKeys is the table order the file is written in.
func langKeys() ([]string, error) {
	b, err := langFiles.ReadFile("assets/lang/keys.txt")
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			keys = append(keys, l)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("keys.txt is empty")
	}
	return keys, nil
}

func langTableHash(keys []string) uint32 {
	h := uint32(2166136261)
	for _, k := range keys {
		for _, c := range []byte(k + "\n") {
			h = (h ^ uint32(c)) * 16777619
		}
	}
	return h
}

func langLoad(code string) (map[string]string, error) {
	b, err := langFiles.ReadFile("assets/lang/" + code + ".json")
	if err != nil {
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s.json: %v", code, err)
	}
	return m, nil
}

// buildLangDat compiles LANG.DAT. problems lists translations left out (bad specifiers, characters the
// menu's fonts lack) and keys a language does not translate are simply absent (the menu shows English).
func buildLangDat() (dat []byte, problems []string, err error) {
	keys, err := langKeys()
	if err != nil {
		return nil, nil, err
	}
	en, err := langLoad("en")
	if err != nil {
		return nil, nil, err
	}
	codes := make([]string, 0, len(langCodes))
	for c := range langCodes {
		codes = append(codes, c)
	}
	sort.Strings(codes)
	type block struct {
		code byte
		data []byte
	}
	var blocks []block
	for _, code := range codes {
		m, err := langLoad(code)
		if err != nil {
			return nil, nil, err
		}
		offsets := make([]uint32, len(keys))
		var strs bytes.Buffer
		for i, k := range keys {
			v := m[k]
			if v == "" {
				continue
			}
			if !langSpecMatch(v, en[k]) {
				problems = append(problems, fmt.Sprintf("%s %s: specifiers differ from the English", code, k))
				continue
			}
			if bad := langUnsupported(v); bad != "" {
				problems = append(problems, fmt.Sprintf("%s %s: the fonts lack %q", code, k, bad))
				continue
			}
			offsets[i] = uint32(4*len(keys) + strs.Len())
			strs.WriteString(v)
			strs.WriteByte(0)
		}
		var b bytes.Buffer
		for _, o := range offsets {
			binary.Write(&b, binary.LittleEndian, o)
		}
		b.Write(strs.Bytes())
		blocks = append(blocks, block{langCodes[code], b.Bytes()})
	}
	var out bytes.Buffer
	out.WriteString("SWL1")
	binary.Write(&out, binary.LittleEndian, langTableHash(keys))
	binary.Write(&out, binary.LittleEndian, uint32(len(keys)))
	binary.Write(&out, binary.LittleEndian, uint32(len(blocks)))
	off := 16 + 16*len(blocks)
	for _, bl := range blocks {
		out.Write([]byte{bl.code, 0, 0, 0})
		binary.Write(&out, binary.LittleEndian, uint32(off))
		binary.Write(&out, binary.LittleEndian, uint32(len(bl.data)))
		binary.Write(&out, binary.LittleEndian, uint32(0))
		off += len(bl.data)
	}
	for _, bl := range blocks {
		out.Write(bl.data)
	}
	return out.Bytes(), problems, nil
}

func langSpecMatch(a, b string) bool {
	return strings.Join(langSpec.FindAllString(a, -1), ",") == strings.Join(langSpec.FindAllString(b, -1), ",")
}

// langUnsupported returns the characters of s outside Latin-1 (the range the menu's fonts hold), or "".
func langUnsupported(s string) string {
	var bad []rune
	for _, r := range s {
		if r > 0xFF {
			bad = append(bad, r)
		}
	}
	return string(bad)
}

// addLanguages writes LANG.DAT into the menu disc's files at every Update SWIRL, so the card's translations
// always match the menu it carries.
func addLanguages(data string, log Logger) {
	dat, problems, err := buildLangDat()
	if err != nil {
		log("The language file was not made (%v); the menu shows English", err)
		return
	}
	for _, p := range problems {
		log("Translation left out: %s", p)
	}
	if err := placeOnDisc(data, langOnDisc, dat); err != nil {
		log("The language file could not be added: %v", err)
	}
}
