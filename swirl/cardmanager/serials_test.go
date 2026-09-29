package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFixSerial(t *testing.T) {
	for _, tc := range []struct{ product, date, name, want string }{
		{"MK51035", "20000120", "CRAZY TAXI", "MK5103550"},
		{"MK51035", "20000124", "CRAZY TAXI", "MK51035"}, // the USA disc keeps its code
		{"T0009M", "", "FIST OF THE NORTH STAR", "T0026M"},
		{"T0009M", "", "RUMBLE FISH", "T0009M"},
		{"T13008N", "20010402", "SPIDER-MAN", "T13011D50"},
		{"T1401N", "20000101", "SONIC", "T1401N"},
		{"", "", "", ""},
	} {
		if got := fixSerial(tc.product, tc.date, tc.name); got != tc.want {
			t.Errorf("fixSerial(%q, %q, %q) = %q, want %q", tc.product, tc.date, tc.name, got, tc.want)
		}
	}
	if serialTitle("MK5105250") != "Skies of Arcadia (PAL)" || serialTitle("T1401N") != "" {
		t.Fatal("serialTitle")
	}
	if len(serialFixes) != 14 {
		t.Fatalf("%d rows in serials.tsv", len(serialFixes))
	}
}

// The table is the same 14 pairs the menu applies at boot (backend/gd_list.c fix_sega_serials), read
// out of the C source so the two cannot drift apart unnoticed.
func TestSerialTableMatchesMenu(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "backend", "gd_list.c"))
	if err != nil {
		t.Skip("backend/gd_list.c not next to this package")
	}
	body := string(src)
	i := strings.Index(body, "static void fix_sega_serials")
	if i < 0 {
		t.Skip("fix_sega_serials not in gd_list.c any more")
	}
	body = body[i:]
	if j := strings.Index(body, "\n}\n"); j > 0 {
		body = body[:j]
	}
	dateRe := regexp.MustCompile(`strcmp\(item->product, "([^"]+)"\) && !strcmp\(item->date, "([^"]+)"\)\)\s*\{\s*strcpy\(item->product, "([^"]+)"\)`)
	nameRe := regexp.MustCompile(`strcmp\(item->product, "([^"]+)"\) && strstr\(item->name, "([^"]+)"\)\)\s*\{\s*strcpy\(item->product, "([^"]+)"\)`)
	menu := map[string]string{}
	for _, m := range dateRe.FindAllStringSubmatch(body, -1) {
		menu[m[1]+"|"+m[2]+"|"] = m[3]
	}
	for _, m := range nameRe.FindAllStringSubmatch(body, -1) {
		menu[m[1]+"||"+m[2]] = m[3]
	}
	serialFixOnce.Do(loadSerialFixes)
	table := map[string]string{}
	for _, fx := range serialFixes {
		table[fx.Product+"|"+fx.Date+"|"+fx.NameHas] = fx.Fixed
	}
	if len(menu) == 0 {
		t.Fatal("no pairs read from gd_list.c; the parser needs updating")
	}
	for k, v := range menu {
		if table[k] != v {
			t.Errorf("gd_list.c fixes %s to %s, serials.tsv says %q", k, v, table[k])
		}
	}
	for k, v := range table {
		if menu[k] != v {
			t.Errorf("serials.tsv fixes %s to %s, gd_list.c says %q", k, v, menu[k])
		}
	}
}

// UP-6: the card stores art and edits under the code the console will use; an edit an older Card Manager
// saved under the raw code is moved over at the next scan.
func TestScanUsesFixedSerial(t *testing.T) {
	root := txnCard(t, "swirl", 1)
	writeTestGDIDated(t, filepath.Join(root, "03"), "CRAZY TAXI", "MK-51035", "20000120")
	writeTestGDIDated(t, filepath.Join(root, "04"), "CRAZY TAXI", "MK-51035", "20000124")
	edits := loadEdits(root)
	edits.Games["03"] = &GameEdit{Product: "MK51035", Region: "E"}
	edits.save()
	c, err := ScanCard(root)
	if err != nil {
		t.Fatal(err)
	}
	if g := findGame(c, "03"); g.Product != "MK5103550" || !g.Edited || g.Region != "E" {
		t.Fatalf("PAL disc: product %q edited %v region %q", g.Product, g.Edited, g.Region)
	}
	if g := findGame(c, "04"); g.Product != "MK51035" {
		t.Fatalf("USA disc: product %q", g.Product)
	}
	if e := loadEdits(root).Games["03"]; e == nil || e.Product != "MK5103550" {
		t.Fatalf("the edit was not moved to the fixed code: %+v", e)
	}
	// serial.txt from GDMENUCardManager holds the raw code; it is fixed the same way
	os.WriteFile(filepath.Join(root, "04", "serial.txt"), []byte("MK-51035"), 0o644)
	os.WriteFile(filepath.Join(root, "03", "serial.txt"), []byte("MK-51035"), 0o644)
	c, _ = ScanCard(root)
	if findGame(c, "03").Product != "MK5103550" || findGame(c, "04").Product != "MK51035" {
		t.Fatalf("with serial.txt: %s %s", findGame(c, "03").Product, findGame(c, "04").Product)
	}
	if err := InstallSwirl(root, "", quiet); err != nil {
		t.Fatal(err)
	}
	ini, _ := menuINI(root)
	if !strings.Contains(ini, "03.product=MK5103550") || !strings.Contains(ini, "04.product=MK51035") {
		t.Fatalf("INI:\n%s", ini)
	}
}

// writeTestGDIDated is writeTestGDI with a release date in the header.
func writeTestGDIDated(t *testing.T, dir, title, serial, date string) {
	t.Helper()
	writeTestGDI(t, dir, title, serial)
	p := filepath.Join(dir, "track03.bin")
	b, _ := os.ReadFile(p)
	copy(b[16+0x50:], date)
	os.WriteFile(p, b, 0o644)
}
