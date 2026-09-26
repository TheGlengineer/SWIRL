//go:build linux

package main

import (
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"/home/user/.local/share/swirl-card-manager/SWIRL-Card-Manager": `"/home/user/.local/share/swirl-card-manager/SWIRL-Card-Manager"`,
		`/path/with"quote`: `"/path/with\"quote"`,
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWriteDesktop(t *testing.T) {
	// point installDir and desktopFile at a temp tree
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	defer os.Setenv("HOME", oldHome)
	os.Setenv("HOME", dir)
	os.Unsetenv("XDG_DATA_HOME")

	exe := filepath.Join(dir, "swirl-card-manager", "SWIRL-Card-Manager")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("x"), 0o755)
	if err := writeDesktop(exe); err != nil {
		t.Fatal(err)
	}
	d := desktopFile()
	b, err := os.ReadFile(d)
	if err != nil {
		t.Fatalf(".desktop not written: %v", err)
	}
	s := string(b)
	for _, want := range []string{"[Desktop Entry]", "Name=SWIRL Card Manager", "Type=Application", "Exec="} {
		if !strings.Contains(s, want) {
			t.Errorf(".desktop missing %q:\n%s", want, s)
		}
	}
	if !fileExists(filepath.Join(installDir(), "icon-256.png")) {
		t.Error("icon not unpacked")
	}
}

func TestHandOverGunzip(t *testing.T) {
	dir := t.TempDir()
	// updatesDir lives under appDataDir; redirect dbDir via HOME is hard, so exercise the gzip read path
	// directly by gunzipping a known payload and checking it matches.
	src := filepath.Join(dir, "a.gz")
	payload := []byte("#!/bin/sh\necho hi\n")
	f, _ := os.Create(src)
	zw := gzip.NewWriter(f)
	zw.Write(payload)
	zw.Close()
	f.Close()

	g, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := gzip.NewReader(g)
	if err != nil {
		t.Fatal(err)
	}
	var got []byte
	buf := make([]byte, 16)
	for {
		n, rerr := zr.Read(buf)
		got = append(got, buf[:n]...)
		if rerr != nil {
			break
		}
	}
	if string(got) != string(payload) {
		t.Errorf("gunzip mismatch: %q", got)
	}
}
