package main

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	// tests never download openMenu's theme; headerlogo_test.go serves its own
	headerLogoZipURL = ""
	os.Exit(m.Run())
}
