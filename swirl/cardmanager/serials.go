package main

// Serial fixes from assets/serials.tsv: a few discs carry another game's product code in IP.BIN, and
// openMenu (backend/gd_list.c, fix_sega_serials) and the Virtual Folder Bundle both correct them on the
// product plus the release date. The card has to use the same corrected codes, or box art, info and
// edits stored under the raw code never match on the console.

import (
	"bufio"
	_ "embed"
	"strings"
	"sync"
)

//go:embed assets/serials.tsv
var serialsTSV string

type serialFix struct {
	Product, Date, NameHas, Fixed, Title string
}

var (
	serialFixOnce sync.Once
	serialFixes   []serialFix
)

func loadSerialFixes() {
	sc := bufio.NewScanner(strings.NewReader(serialsTSV))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) < 4 || f[0] == "" || f[3] == "" {
			continue
		}
		fx := serialFix{Product: f[0], Date: f[1], NameHas: f[2], Fixed: f[3]}
		if len(f) > 4 {
			fx.Title = f[4]
		}
		serialFixes = append(serialFixes, fx)
	}
}

// fixSerial returns the product code the menu will use for a disc: the corrected one when the raw code,
// date and title match a row of the table, else the code as given.
func fixSerial(product, date, name string) string {
	serialFixOnce.Do(loadSerialFixes)
	for _, fx := range serialFixes {
		if fx.Product != product {
			continue
		}
		if fx.Date != "" && fx.Date != date {
			continue
		}
		if fx.NameHas != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(fx.NameHas)) {
			continue
		}
		return fx.Fixed
	}
	return product
}

// serialTitle names the game behind a corrected product code ("" when it is not in the table).
func serialTitle(product string) string {
	serialFixOnce.Do(loadSerialFixes)
	for _, fx := range serialFixes {
		if fx.Fixed == product {
			return fx.Title
		}
	}
	return ""
}
