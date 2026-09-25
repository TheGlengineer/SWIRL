package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/nwaples/rardecode/v2"
)

// Solid RAR archives whose first file is small used to decode wrongly (see third_party/rardecode/reader.go).
func TestRarSolidSmallFirstFile(t *testing.T) {
	if _, err := exec.LookPath("rar"); err != nil {
		t.Skip("rar not installed")
	}
	src := t.TempDir()
	writeTestGDI(t, filepath.Join(src, "pack", "One"), "ONE", "T-00030N")
	writeTestCDI(t, filepath.Join(src, "pack", "Two", "two.cdi"), "TWO", "T-00031N")
	out := filepath.Join(src, "x.rar")
	if b, err := exec.Command("rar", "a", "-r", "-ep1", "-idq", "-s", out, filepath.Join(src, "pack")+string(os.PathSeparator)+"*").CombinedOutput(); err != nil {
		t.Fatal(string(b), err)
	}
	r, err := rardecode.OpenReader(out)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, r); err != nil {
			t.Fatal(h.Name, err)
		}
	}
}
