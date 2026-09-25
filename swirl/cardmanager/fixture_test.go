package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// SWIRL_FIXTURES=dir go test -run TestMakeFixtures builds a folder of sample downloads for trying the file browser.
func TestMakeFixtures(t *testing.T) {
	dir := os.Getenv("SWIRL_FIXTURES")
	if dir == "" {
		t.Skip()
	}
	os.MkdirAll(dir, 0o755)
	w := filepath.Join(t.TempDir())
	writeTestGDI(t, filepath.Join(w, "Zip Racer (USA)"), "ZIP RACER", "T-00010N")
	zipDir(t, filepath.Join(w, "Zip Racer (USA)"), filepath.Join(dir, "Zip Racer (USA).zip"))
	writeTestCDI(t, filepath.Join(w, "s", "Seven.cdi"), "SEVEN", "T-00020N")
	exec.Command("python3", "-c", `import py7zr,sys; a=py7zr.SevenZipFile(sys.argv[1],'w'); a.write(sys.argv[2],'Seven.cdi'); a.close()`, filepath.Join(dir, "Seven Game (Europe).7z"), filepath.Join(w, "s", "Seven.cdi")).Run()
	writeTestGDI(t, filepath.Join(w, "pack", "Rar One"), "RAR ONE", "T-00030N")
	writeTestCDI(t, filepath.Join(w, "pack", "Rar Two", "two.cdi"), "RAR TWO", "T-00031N")
	exec.Command("rar", "a", "-s", "-r", "-ep1", "-idq", filepath.Join(dir, "Dreamcast Pack.rar"), filepath.Join(w, "pack")+"/*").Run()
	writeTestGDI(t, filepath.Join(dir, "Crazy Folder Game"), "CRAZY FOLDER", "T-00040N")
	writeTestCDI(t, filepath.Join(dir, "Loose Game.cdi"), "LOOSE", "T-00050N")
	writeTestCDI(t, filepath.Join(dir, "Already There", "x.cdi"), "18WHEELER", "MK-51064")
	os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "broken.zip"), []byte("not a zip"), 0o644)
	os.MkdirAll(filepath.Join(dir, "More downloads"), 0o755)
}
