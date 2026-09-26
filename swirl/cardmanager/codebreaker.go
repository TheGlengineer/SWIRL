package main

// CodeBreaker. SWIRL can start a game through CodeBreaker (the cheat disc's PELICAN.BIN) with its cheats, but
// SWIRL can't include CodeBreaker. The owner picks their own PELICAN.BIN once; Card Manager keeps it in the
// card's SWIRL folder with any cheat files found next to it, and puts them on the menu disc at every Update
// SWIRL, where SWIRL's launch options offer "Play with CodeBreaker cheats".

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func cbDir(root string) string { return filepath.Join(root, editsDir, "CODEBREAKER") }

type CodeBreakerInfo struct {
	Present bool  `json:"present"`
	Bytes   int64 `json:"bytes"`
	Cheats  int   `json:"cheats"`
}

func GetCodeBreakerInfo(root string) CodeBreakerInfo {
	var ci CodeBreakerInfo
	if st, err := os.Stat(filepath.Join(cbDir(root), "PELICAN.BIN")); err == nil {
		ci.Present, ci.Bytes = true, st.Size()
	}
	for _, e := range listDir(filepath.Join(cbDir(root), "CHEATS")) {
		if strings.EqualFold(filepath.Ext(e), ".bin") {
			ci.Cheats++
		}
	}
	return ci
}

// checkPelican refuses files that clearly are not CodeBreaker's program.
func checkPelican(b []byte) error {
	switch {
	case len(b) < 16<<10:
		return errors.New("this file is too small to be CodeBreaker's PELICAN.BIN")
	case len(b) > 16<<20:
		return errors.New("this file is too large to be CodeBreaker's PELICAN.BIN; pick the PELICAN.BIN file, not a disc image")
	case bytes.Contains(b, []byte("SWIRL: %d games")):
		return errors.New("this is SWIRL's own program, not CodeBreaker")
	case bytes.HasPrefix(b, []byte("SEGA SEGAKATANA")) || bytes.Contains(b[:min(len(b), 64<<10)], []byte("SEGA SEGAKATANA")):
		return errors.New("this looks like a disc image; pick the PELICAN.BIN file from it")
	}
	zero := true
	for _, c := range b[:4096] {
		if c != 0 {
			zero = false
			break
		}
	}
	if zero {
		return errors.New("this file is empty")
	}
	return nil
}

// SetCodeBreaker keeps the owner's PELICAN.BIN and the cheat files found next to it (a CHEATS folder, or
// FCDCHEATS.BIN beside it).
func SetCodeBreaker(root, src string, log Logger) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("could not open %s", src)
	}
	if err := checkPelican(b); err != nil {
		return err
	}
	dir := cbDir(root)
	tmp := dir + ".new"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(filepath.Join(tmp, "CHEATS"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "PELICAN.BIN"), b, 0o644); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	cheats := 0
	from := filepath.Dir(src)
	add := func(p, name string) {
		if c, err := os.ReadFile(p); err == nil && len(c) > 0 && len(c) < 4<<20 {
			if os.WriteFile(filepath.Join(tmp, "CHEATS", strings.ToUpper(name)), c, 0o644) == nil {
				cheats++
			}
		}
	}
	if d := findCI(from, "CHEATS"); d != "" {
		for _, e := range listDir(d) {
			if strings.EqualFold(filepath.Ext(e), ".bin") {
				add(filepath.Join(d, e), e)
			}
		}
	}
	if p := findCI(from, "FCDCHEATS.BIN"); p != "" && cheats == 0 {
		add(p, "FCDCHEATS.BIN")
	}
	os.RemoveAll(dir)
	if err := os.Rename(tmp, dir); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	if cheats > 0 {
		log("Kept CodeBreaker with %d cheat files. Click Update SWIRL to put it on the card.", cheats)
	} else {
		log("Kept CodeBreaker (no cheat files were next to it; CodeBreaker's own list is used). Click Update SWIRL to put it on the card.")
	}
	return nil
}

func RemoveCodeBreaker(root string) error {
	return os.RemoveAll(cbDir(root))
}

// addCodeBreaker puts the owner's CodeBreaker on the menu disc being built. Without one, a CodeBreaker the old
// menu disc already had is left as it is.
func addCodeBreaker(root, data string, log Logger) error {
	if !GetCodeBreakerInfo(root).Present {
		return nil
	}
	for _, e := range listDir(data) {
		if strings.EqualFold(e, "PELICAN.BIN") || strings.EqualFold(e, "CHEATS") {
			os.RemoveAll(filepath.Join(data, e))
		}
	}
	if err := copyFile(filepath.Join(cbDir(root), "PELICAN.BIN"), filepath.Join(data, "PELICAN.BIN")); err != nil {
		return err
	}
	n := 0
	for _, e := range listDir(filepath.Join(cbDir(root), "CHEATS")) {
		os.MkdirAll(filepath.Join(data, "CHEATS"), 0o755)
		if err := copyFile(filepath.Join(cbDir(root), "CHEATS", e), filepath.Join(data, "CHEATS", e)); err != nil {
			return err
		}
		n++
	}
	log("Added CodeBreaker (%d cheat files)", n)
	return nil
}
