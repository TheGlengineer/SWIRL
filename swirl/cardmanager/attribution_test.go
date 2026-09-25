package main

import (
	"bytes"
	"strings"
	"testing"
)

// Every SWIRL that Card Manager installs must carry the author credit shown in System > About SWIRL.
func TestSwirlCarriesAttribution(t *testing.T) {
	for _, s := range []string{"Created by Glen Huszar", "github.com/TheGlengineer", "Built on openMenu by mrneo240"} {
		if !bytes.Contains(swirlBinary, []byte(s)) {
			t.Fatalf("the embedded SWIRL menu is missing %q", s)
		}
	}
}

// The menu and Card Manager share one version number, shown in System > About SWIRL.
func TestSwirlVersionMatches(t *testing.T) {
	// Card Manager patch releases (2.6.1) that do not touch the menu keep the menu's 2.6
	mv := version
	if parts := strings.SplitN(version, ".", 3); len(parts) == 3 {
		mv = parts[0] + "." + parts[1]
	}
	if !bytes.Contains(swirlBinary, []byte("Version "+mv+"\x00")) {
		t.Fatalf("the embedded SWIRL menu does not report version %s; update ui/swirl/sw_version.h and rebuild it", mv)
	}
}

// About shows the SWIRL version found on the card.
func TestSwirlReleaseRead(t *testing.T) {
	mv := strings.Join(strings.SplitN(version, ".", 3)[:2], ".")
	if got := swirlRelease(swirlBinary); got != mv {
		t.Fatalf("swirlRelease = %q, want %q", got, mv)
	}
}
