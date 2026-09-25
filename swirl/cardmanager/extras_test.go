package main

import (
	"os"
	"testing"
)

// EXTRAS_CARD=<card root> EXTRAS_TUNE=<file.wav> go test -run TestExtrasOnCard
func TestExtrasOnCard(t *testing.T) {
	root := os.Getenv("EXTRAS_CARD")
	if root == "" {
		t.Skip()
	}
	if _, err := SetMusic(root, os.Getenv("EXTRAS_TUNE"), t.Logf); err != nil {
		t.Fatal(err)
	}
	if err := SaveCollections(root, []Collection{{Name: "Couch co-op", Products: []string{"MK51049", "T1401N", "T3601N", "T9704N"}}, {Name: "Glen's picks", Products: []string{"MK51035", "MK51000", "MK51058"}}}); err != nil {
		t.Fatal(err)
	}
	if err := StartShotDownload(root); err != nil {
		t.Fatal(err)
	}
	j := waitJobLong(t)
	t.Log(j.Log)
	if err := installSwirl(root, "", false, t.Logf); err != nil {
		t.Fatal(err)
	}
	rep, err := CheckCard(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range rep.Items {
		t.Logf("health %s %s %s", it.Level, it.Folder, it.Message)
	}
}
