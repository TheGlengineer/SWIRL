package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestZipTreeKeepsModesAndLinks(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, mode os.FileMode, body string) {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate}
		h.SetMode(mode)
		w, _ := zw.CreateHeader(h)
		w.Write([]byte(body))
	}
	add("Test.app/", os.ModeDir|0o755, "")
	add("Test.app/Contents/MacOS/Test", 0o755, "#!/bin/sh\n")
	add("Test.app/Contents/Info.plist", 0o644, "<plist/>")
	add("Test.app/Contents/Frameworks/X.framework/Versions/A/X", 0o755, "bin")
	add("Test.app/Contents/Frameworks/X.framework/Versions/Current", os.ModeSymlink|0o755, "A")
	zw.Close()
	zr, _ := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	dst := t.TempDir()
	if err := extractZipTree(zr, dst); err != nil {
		t.Fatal(err)
	}
	app := findApp(dst)
	st, err := os.Stat(filepath.Join(app, "Contents", "MacOS", "Test"))
	if err != nil || st.Mode().Perm()&0o100 == 0 {
		t.Fatal("executable bit lost", err)
	}
	if l, err := os.Readlink(filepath.Join(app, "Contents/Frameworks/X.framework/Versions/Current")); err != nil || l != "A" {
		t.Fatal("link lost", l, err)
	}
	// copy keeps both too
	cp := filepath.Join(t.TempDir(), "Copy.app")
	if err := copyTreeKeep(app, cp); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(filepath.Join(cp, "Contents", "MacOS", "Test")); st.Mode().Perm()&0o100 == 0 {
		t.Fatal("copy lost the executable bit")
	}
	if l, _ := os.Readlink(filepath.Join(cp, "Contents/Frameworks/X.framework/Versions/Current")); l != "A" {
		t.Fatal("copy lost the link")
	}
	// escaping links are refused
	var bad bytes.Buffer
	zw = zip.NewWriter(&bad)
	h := &zip.FileHeader{Name: "a/link"}
	h.SetMode(os.ModeSymlink | 0o777)
	w, _ := zw.CreateHeader(h)
	w.Write([]byte("../../etc"))
	zw.Close()
	zr, _ = zip.NewReader(bytes.NewReader(bad.Bytes()), int64(bad.Len()))
	if extractZipTree(zr, t.TempDir()) == nil {
		t.Fatal("escaping link accepted")
	}
}
