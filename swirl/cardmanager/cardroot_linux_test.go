//go:build linux

package main

import "testing"

func TestCardRootErr(t *testing.T) {
	if cardRootErr("/") == "" {
		t.Fatal("root was not rejected")
	}
	if cardRootErr("/home/james/SWIRL") != "" {
		t.Fatal("a normal path was rejected")
	}
	if cardRootErr("") != "" {
		t.Fatal("empty path rejected (should be handled by is-not-a-folder)")
	}
}
