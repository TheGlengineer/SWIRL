package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestParseDatSkipsEmptyAndOutsideEntries(t *testing.T) {
	d := newDat(64)
	d.Set("GOOD1", []byte("one"))
	d.Set("GOOD2", []byte("two"))
	p := filepath.Join(t.TempDir(), "X.DAT")
	if err := d.Write(p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	// forge a third entry with an empty ID and a fourth outside the file
	hdr := make([]byte, 16+16*4)
	copy(hdr, b[:16+16*2])
	binary.LittleEndian.PutUint32(hdr[8:], 4)
	binary.LittleEndian.PutUint32(hdr[16+16*2+12:], 2) // empty ID, valid chunk
	copy(hdr[16+16*3:], "FAR")
	binary.LittleEndian.PutUint32(hdr[16+16*3+12:], 99)
	forged := append(append([]byte(nil), hdr...), b[64:]...)
	d2, err := parseDat(forged)
	if err != nil {
		t.Fatal(err)
	}
	if len(d2.Order) != 2 || d2.Order[0] != "GOOD1" || d2.Order[1] != "GOOD2" {
		t.Fatalf("order %v", d2.Order)
	}
}
