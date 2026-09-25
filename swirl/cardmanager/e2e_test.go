package main

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestEndToEnd(t *testing.T) {
	root := os.Getenv("SWIRL_CARD")
	if root == "" {
		t.Skip("set SWIRL_CARD")
	}
	logf := func(f string, a ...any) { t.Logf(f, a...) }
	// a CDI game with disc art in folder 08
	dir := t.TempDir()
	data := filepath.Join(dir, "d")
	os.MkdirAll(data, 0o755)
	os.WriteFile(filepath.Join(data, "1ST_READ.BIN"), make([]byte, 5000), 0o644)
	os.WriteFile(filepath.Join(data, "0GDTEX.PVR"), encodeVQ(quadImage(256)), 0o644)
	iso := filepath.Join(dir, "s.iso")
	buildISO(data, iso, 11702, "T", ipSector("CDI ART GAME", "T-99901N"))
	raw, _ := os.ReadFile(iso)
	g := filepath.Join(root, "08")
	os.MkdirAll(g, 0o755)
	f, _ := os.Create(filepath.Join(g, "game.cdi"))
	f.Write(make([]byte, 2352*300))
	for i := 0; i < len(raw)/2048; i++ {
		f.Write(make([]byte, 8))
		f.Write(raw[i*2048 : (i+1)*2048])
		f.Write(make([]byte, 280))
	}
	f.Close()

	n, err := FillFromDiscs(root, logf)
	if err != nil || n < 1 {
		t.Fatal("fill", n, err)
	}
	var pb bytes.Buffer
	png.Encode(&pb, quadImage(300))
	err = SaveGame(SaveGameRequest{Root: root, Folder: "02", Name: "SONIC ADVENTURE DX", Serial: "SWT-0001", Region: "U", VGA: true, Date: "19990909",
		Meta: Meta{Players: 1, VMUBlocks: 4, Genre: 512, Description: "Edited in SWIRL Card Manager"},
		Box:  "data:image/png;base64," + base64.StdEncoding.EncodeToString(pb.Bytes()), VMUArt: true})
	if err != nil {
		t.Fatal(err)
	}
	det, err := GetGameDetail(root, "02")
	if err != nil || det.Meta.Description != "Edited in SWIRL Card Manager" || det.Game.Name != "SONIC ADVENTURE DX" {
		t.Fatalf("detail %+v %v", det, err)
	}
	if err := InstallSwirl(root, "", logf); err != nil {
		t.Fatal(err)
	}
	m, err := openMenuDisc(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	mb, err := m.datChunk("META.DAT", "SWT0001")
	if err != nil || decodeMeta(mb).Description != "Edited in SWIRL Card Manager" {
		t.Fatal("meta after install", err)
	}
	if _, err := m.datChunk("VMU.DAT", "SWT0001"); err != nil {
		t.Fatal("vmu", err)
	}
	if _, err := m.datChunk("VMU.DAT", "T99901N"); err != nil {
		t.Fatal("vmu cdi", err)
	}
	bb, err := m.datChunk("BOX.DAT", "T99901N")
	if err != nil {
		t.Fatal("box cdi", err)
	}
	if img, err := decodePVR(bb); err != nil || img.NRGBAAt(200, 200).R < 240 {
		t.Fatal("box decode", err)
	}
	// old art still there for a game we did not touch
	if _, err := m.datChunk("BOX.DAT", "MK51064"); err != nil {
		t.Log("note: MK51064 not in this card's BOX.DAT")
	}
	for _, k := range []string{"box", "vmu"} {
		if _, err := GameArt(root, "02", k); err != nil {
			t.Fatal("art", k, err)
		}
	}
	c, _ := ScanCard(root)
	t.Logf("games %d, menu %s", len(c.Games), c.MenuType)
}
