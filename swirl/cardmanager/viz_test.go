package main

import (
	"os"
	"path/filepath"
	"testing"
)

// SWIRL_CARD=card root go test -run TestViz (writes box_*.png and vmu_*.png to the temp folder)
func TestViz(t *testing.T) {
	root := os.Getenv("SWIRL_CARD")
	if root == "" {
		t.Skip()
	}
	m, err := openMenuDisc(root)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	for _, id := range []string{"MK51064", "T9708N"} {
		b, err := m.datChunk("BOX.DAT", id)
		if err != nil {
			t.Log(id, err)
			continue
		}
		img, err := decodePVR(b)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(os.TempDir(), "box_"+id+".png"), pngBytes(img), 0o644)
		os.WriteFile(filepath.Join(os.TempDir(), "vmu_"+id+".png"), vmuPNG(makeVMU(img, false)), 0o644)
	}
}
