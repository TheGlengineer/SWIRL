package main

import (
	"os"
	"strings"
	"testing"
)

func TestThumbMatching(t *testing.T) {
	// SWIRL_THUMB_LIST=file listing libretro-thumbnails paths, one per line
	b, err := os.ReadFile(os.Getenv("SWIRL_THUMB_LIST"))
	if err != nil {
		t.Skip("set SWIRL_THUMB_LIST")
	}
	idx := map[string][]thumbEntry{}
	for _, p := range strings.Split(string(b), "\n") {
		dir, file, ok := strings.Cut(p, "/")
		if !ok || !strings.HasSuffix(file, ".png") {
			continue
		}
		title := strings.TrimSuffix(file, ".png")
		base := parenRe.ReplaceAllString(title, "")
		main := base
		if i := strings.Index(base, " - "); i > 0 {
			main = base[:i]
		}
		idx[dir] = append(idx[dir], thumbEntry{file: file, base: normTitle(base), main: normTitle(main)})
	}
	names := []string{"18WHEELER", "4WHEELTHUNDER", "AEROWINGS", "AEROWINGS 2 AIR STRIKE", "AIRFORCE DELTA", "ARMADA", "CHUCHU ROCKET", "CRAZY TAXI", "CRAZY TAXI 2", "DAYTONAUSA", "DEAD OR ALIVE 2", "DEEP FIGHTER", "DINO CRISIS", "GRAND THEFT AUTO 2", "HEAVY METAL GEOMATRIX", "HYDRO THUNDER", "INCOMING", "IRON ACES", "JET GRIND RADIO", "METROPOLIS STREET RACER", "MORTAL KOMBAT GOLD", "POWER STONE 2 USA", "RAINBOW SIX", "READY 2 RUMBLE BOXING", "READY 2 RUMBLE BOXING ROUND 2", "RESIDENT EVIL 2", "RESIDENT EVIL CODE VERONICA", "RESIDENT EVIL3", "ROGUESPEAR", "SEGA RALLY 2", "SEGAGT", "SILENT SCOPE", "SONIC ADVENTURE", "SONIC ADVENTURE 2", "SOULCALIBUR", "STAR WARS EPISODE 1 RACER", "STARLANCER", "TEST DRIVE 6", "TEST DRIVE LE MANS", "THE HOUSE OF THE DEAD 2", "TOKYO XTREME RACER", "TOKYO XTREME RACER 2", "TOMB RAIDER THE LAST REVELATION", "TOY COMMANDER", "VF3TB", "ZOMBIE REVENGE"}
	miss := 0
	for _, n := range names {
		f := bestThumb(n, idx["Named_Snaps"])
		if f == "" {
			miss++
		}
		t.Logf("%-32s -> %s", n, f)
	}
	t.Logf("missing %d of %d", miss, len(names))
}
