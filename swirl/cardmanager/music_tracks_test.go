package main

import (
	"os"
	"path/filepath"
	"testing"
)

// a tiny OMBG file: header plus a few payload bytes
func fakeBGM(rate, channels int) []byte {
	h := make([]byte, 32)
	copy(h, "OMBG")
	h[4] = 1
	h[8], h[9], h[10] = byte(rate), byte(rate>>8), byte(rate>>16)
	h[12] = byte(channels)
	return append(h, make([]byte, 64)...)
}

func writeTrackSource(t *testing.T, dir, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestMusicTracks(t *testing.T) {
	root := t.TempDir()
	src := t.TempDir()
	quiet := func(string, ...interface{}) {}
	a := writeTrackSource(t, src, "first.adp", fakeBGM(44100, 2))
	b := writeTrackSource(t, src, "second.adp", fakeBGM(44100, 2))
	c := writeTrackSource(t, src, "third.adp", fakeBGM(44100, 2))
	for _, p := range []string{a, b, c} {
		if _, err := SetMusic(root, p, quiet); err != nil {
			t.Fatal(err)
		}
	}
	mi := GetMusicInfo(root)
	if !mi.Present || len(mi.Tracks) != 3 || mi.Tracks[0].Name != "first.adp" || mi.Tracks[2].Number != 3 {
		t.Fatalf("after three adds: %+v", mi)
	}
	if !fileExists(filepath.Join(root, editsDir, "BGM.ADP")) || !fileExists(filepath.Join(root, editsDir, "BGM3.ADP")) {
		t.Fatal("tracks are BGM.ADP, BGM2.ADP, BGM3.ADP")
	}
	// a track in another format is refused: the menu plays one format
	mono := writeTrackSource(t, src, "mono.adp", fakeBGM(22050, 1))
	if _, err := SetMusic(root, mono, quiet); err == nil {
		t.Fatal("a 22 kHz mono track was added next to 44.1 kHz stereo ones")
	}
	// removing the middle track closes the gap
	if err := RemoveTrack(root, 2); err != nil {
		t.Fatal(err)
	}
	mi = GetMusicInfo(root)
	if len(mi.Tracks) != 2 || mi.Tracks[0].Name != "first.adp" || mi.Tracks[1].Name != "third.adp" || mi.Tracks[1].Number != 2 {
		t.Fatalf("after removing track 2: %+v", mi.Tracks)
	}
	if fileExists(filepath.Join(root, editsDir, "BGM3.ADP")) {
		t.Fatal("BGM3.ADP should have moved down to BGM2.ADP")
	}
	// the limit
	for i := 0; i < maxMusicTracks-2; i++ {
		if _, err := SetMusic(root, a, quiet); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SetMusic(root, a, quiet); err == nil {
		t.Fatalf("track %d was accepted", maxMusicTracks+1)
	}
	// removing the only track goes back to the theme
	if err := RemoveMusic(root); err != nil {
		t.Fatal(err)
	}
	if mi := GetMusicInfo(root); mi.Present || len(musicTracks(root)) != 0 || !fileExists(themeMarker(root)) {
		t.Fatalf("after RemoveMusic: %+v", mi)
	}
	if _, err := SetMusic(root, a, quiet); err != nil {
		t.Fatal(err)
	}
	if err := RemoveTrack(root, 1); err != nil {
		t.Fatal(err)
	}
	if musicTracks(root) != nil {
		t.Fatal("removing the last track should leave no music")
	}
}
