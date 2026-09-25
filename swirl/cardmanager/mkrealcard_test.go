package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Builds a card that mirrors Glen's game list (names, serials, discs) with small stand-in discs.
// MK_REAL=<empty folder> go test -run TestMakeRealCard
func TestMakeRealCard(t *testing.T) {
	root := os.Getenv("MK_REAL")
	if root == "" {
		t.Skip()
	}
	f, err := os.Open("/mnt/user-data/outputs/card_OPENMENU.INI")
	if err != nil {
		t.Fatal(err)
	}
	vals := map[string]map[string]string{}
	var order []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok || !strings.Contains(k, ".") {
			continue
		}
		slot, key, _ := strings.Cut(k, ".")
		if vals[slot] == nil {
			vals[slot] = map[string]string{}
			order = append(order, slot)
		}
		vals[slot][key] = v
	}
	seen := map[string]bool{}
	n := 2
	for _, s := range order {
		v := vals[s]
		if s == "01" {
			continue
		}
		key := v["product"] + v["disc"]
		if seen[key] {
			continue
		}
		seen[key] = true
		ip := ipSector(v["name"], v["product"])
		copy(ip[0x25:], "GD-ROM"+v["disc"])
		dir := t.TempDir()
		data := filepath.Join(dir, "d")
		os.MkdirAll(data, 0o755)
		os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), make([]byte, 5000), 0o644)
		iso := filepath.Join(dir, "s.iso")
		buildISO(data, iso, 11702, "T", ip)
		raw, _ := os.ReadFile(iso)
		folder := filepath.Join(root, fmt.Sprintf("%02d", n))
		os.MkdirAll(folder, 0o755)
		out, _ := os.Create(filepath.Join(folder, "disc.cdi"))
		out.Write(make([]byte, 2352*300))
		for i := 0; i < len(raw)/2048; i++ {
			out.Write(make([]byte, 8))
			out.Write(raw[i*2048 : (i+1)*2048])
			out.Write(make([]byte, 280))
		}
		out.Close()
		n++
	}
	if err := installSwirl(root, "", false, t.Logf); err != nil {
		t.Fatal(err)
	}
}
