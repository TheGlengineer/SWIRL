package main

import (
	"archive/zip"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestGDI makes a small GDI game whose track 3 carries a real IP.BIN.
func writeTestGDI(t *testing.T, dir, title, serial string) {
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "disc.gdi"), []byte("3\r\n1 0 4 2352 track01.bin 0\r\n2 600 0 2352 track02.raw 0\r\n3 45000 4 2352 track03.bin 0\r\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "track01.bin"), make([]byte, 2352*4), 0o644)
	os.WriteFile(filepath.Join(dir, "track02.raw"), make([]byte, 2352*4), 0o644)
	ip := ipSector(title, serial)
	var raw []byte
	for i := 0; i < 16; i++ {
		sec := make([]byte, 2352)
		copy(sec[16:], ip[(i*2048)%len(ip):])
		raw = append(raw, sec...)
	}
	os.WriteFile(filepath.Join(dir, "track03.bin"), raw, 0o644)
}

func zipDir(t *testing.T, src, out string) {
	f, _ := os.Create(out)
	zw := zip.NewWriter(f)
	filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(filepath.Dir(src), p)
		w, _ := zw.Create(filepath.ToSlash(rel))
		in, _ := os.Open(p)
		io.Copy(w, in)
		in.Close()
		return nil
	})
	zw.Close()
	f.Close()
}

func TestArchivesAdd(t *testing.T) {
	card := t.TempDir()
	writeTestCDI(t, filepath.Join(card, "02", "disc.cdi"), "ON CARD", "T-00001N")
	logf := func(f string, a ...any) { t.Logf(f, a...) }
	if err := installSwirl(card, "", false, logf); err != nil {
		t.Fatal(err)
	}
	src := t.TempDir()

	// zip: a GDI game inside a named folder (serial read without unpacking)
	writeTestGDI(t, filepath.Join(src, "work", "Zip Racer (USA)"), "ZIP RACER", "T-00010N")
	zipDir(t, filepath.Join(src, "work", "Zip Racer (USA)"), filepath.Join(src, "Zip Racer (USA).zip"))

	// 7z: a CDI at the archive root, solid
	writeTestCDI(t, filepath.Join(src, "work7", "Seven.cdi"), "SEVEN", "T-00020N")
	py := exec.Command("python3", "-c", `import py7zr,sys; a=py7zr.SevenZipFile(sys.argv[1],'w'); a.write(sys.argv[2],'Seven.cdi'); a.close()`, filepath.Join(src, "Seven Game.7z"), filepath.Join(src, "work7", "Seven.cdi"))
	if out, err := py.CombinedOutput(); err != nil {
		t.Skip("py7zr not available:", string(out))
	}

	// rar: two games in one solid archive, plus Mac junk
	writeTestGDI(t, filepath.Join(src, "pack", "Rar One"), "RAR ONE", "T-00030N")
	writeTestCDI(t, filepath.Join(src, "pack", "Rar Two", "two.cdi"), "RAR TWO", "T-00031N")
	os.MkdirAll(filepath.Join(src, "pack", "__MACOSX"), 0o755)
	os.WriteFile(filepath.Join(src, "pack", "__MACOSX", "._x.cdi"), []byte("junk"), 0o644)
	rar := exec.Command("rar", "a", "-s", "-r", "-ep1", "-idq", filepath.Join(src, "Pack.rar"), filepath.Join(src, "pack")+string(os.PathSeparator)+"*")
	if out, err := rar.CombinedOutput(); err != nil {
		t.Skip("rar not available:", string(out))
	}

	// a 7z of the game already on the card: only found to be a duplicate after unpacking
	writeTestCDI(t, filepath.Join(src, "work8", "again.cdi"), "ON CARD", "T-00001N")
	exec.Command("python3", "-c", `import py7zr,sys; a=py7zr.SevenZipFile(sys.argv[1],'w'); a.write(sys.argv[2],'again.cdi'); a.close()`, filepath.Join(src, "Again.7z"), filepath.Join(src, "work8", "again.cdi")).Run()

	// peek (what the browser shows)
	pk := Peek(filepath.Join(src, "Zip Racer (USA).zip"), card)
	if len(pk.Games) != 1 || pk.Product != "T-00010N" || pk.Games[0].Name != "Zip Racer (USA)" || pk.OnCard != "" {
		t.Fatalf("peek zip %+v", pk)
	}
	pk = Peek(filepath.Join(src, "Pack.rar"), card)
	if len(pk.Games) != 2 || pk.Games[0].Name != "Rar One" || pk.Games[1].Name != "Rar Two" {
		t.Fatalf("peek rar %+v", pk)
	}
	pk = Peek(filepath.Join(card, "02"), card)
	if pk.OnCard != "02" {
		t.Fatalf("peek card folder %+v", pk)
	}

	// zip duplicate of a zip game already added is skipped before copying
	sources := []string{
		filepath.Join(src, "Zip Racer (USA).zip"),
		filepath.Join(src, "Seven Game.7z"),
		filepath.Join(src, "Pack.rar"),
		filepath.Join(src, "Again.7z"),
		filepath.Join(src, "Zip Racer (USA).zip"),
	}
	if err := StartAddGames(card, sources, ""); err != nil {
		t.Fatal(err)
	}
	j := waitJobLong(t)
	if j.Error != "" {
		t.Fatal(j.Error, j.Log)
	}
	log := strings.Join(j.Log, "\n")
	t.Log(log)
	if !strings.Contains(log, "Skipped Zip Racer (USA): it is the same disc as Zip Racer (USA)") || !strings.Contains(log, "Skipped Again: already on the card in folder 02") {
		t.Fatal("duplicates were not reported")
	}
	c, _ := ScanCard(card)
	var got []string
	for _, g := range c.Games {
		got = append(got, g.Folder+":"+g.Name+":"+g.Product)
	}
	want := "02:ON CARD:T00001N,03:Zip Racer:T00010N,04:Seven Game:T00020N,05:Rar One:T00030N,06:Rar Two:T00031N"
	if strings.Join(got, ",") != want {
		t.Fatalf("card after add:\n%s\nwant\n%s", strings.Join(got, ","), want)
	}
	for _, f := range []string{"03/disc.gdi", "03/track03.bin", "05/track02.raw", "06/two.cdi"} {
		if !fileExists(filepath.Join(card, f)) {
			t.Fatal("missing", f)
		}
	}
	if fileExists(filepath.Join(card, "07")) {
		t.Fatal("the duplicate's folder was left behind")
	}
	a, _ := os.ReadFile(filepath.Join(src, "pack", "Rar One", "track03.bin"))
	b, _ := os.ReadFile(filepath.Join(card, "05", "track03.bin"))
	if string(a) != string(b) {
		t.Fatal("unpacked track differs")
	}
	// everything is a duplicate now
	if _, err := planAdd(card, []string{filepath.Join(src, "Zip Racer (USA).zip")}); err == nil || !strings.Contains(err.Error(), "already on the card") {
		t.Fatal("expected all-duplicates error, got", err)
	}
}

func TestArchiveNames(t *testing.T) {
	if !isArchive("Game.part1.rar") || isArchive("Game.part2.rar") || !isArchivePart("Game.r00") || !isArchive("x.7z") || isArchive("x.gdi") {
		t.Fatal("archive name rules")
	}
	g := gamesInArchive("Shenmue (USA).part1.rar", []arcEntry{{"disc.gdi", 1}, {"track01.bin", 2}})
	if len(g) != 1 || g[0].Name != "Shenmue (USA)" || g[0].Bytes != 3 {
		t.Fatalf("%+v", g)
	}
	g = gamesInArchive("pack.zip", []arcEntry{{"A/x.cdi", 1}, {"A/y.cdi", 1}, {"B/GDI/disc.gdi", 1}, {"B/GDI/track01.bin", 1}})
	if len(g) != 3 || g[0].Name != "x" || g[1].Name != "y" || g[2].Name != "B" {
		t.Fatalf("%+v", g)
	}
}
