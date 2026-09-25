package main

// GDEMU.INI editor. Settings documented by GDEMU's author (gdemu.wordpress.com, "GDEMU operation").
// Only known keys are edited; any other lines on the card are kept as they are.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type GDEMUSettings struct {
	Exists     bool   `json:"exists"`
	OpenTime   string `json:"openTime"`   // 150..5000 ms, empty = GDEMU default (500)
	DetectTime string `json:"detectTime"` // 150..5000 ms, empty = default (250)
	ResetGoto  string `json:"resetGoto"`  // 0..9999, empty or 0 = off
	ImageTests string `json:"imageTests"` // 0 or 1, empty = default (on)
	HighSpeed  string `json:"highSpeed"`  // 0 or 1, empty = default (off)
	ReadLimit  string `json:"readLimit"`  // -1, 0 or 600..1250, empty = default (automatic)
	Other      string `json:"other"`      // lines SWIRL does not manage
}

var gdemuKeys = []string{"open_time", "detect_time", "reset_goto", "image_tests", "high_speed", "read_limit"}

func (s *GDEMUSettings) field(key string) *string {
	switch key {
	case "open_time":
		return &s.OpenTime
	case "detect_time":
		return &s.DetectTime
	case "reset_goto":
		return &s.ResetGoto
	case "image_tests":
		return &s.ImageTests
	case "high_speed":
		return &s.HighSpeed
	case "read_limit":
		return &s.ReadLimit
	}
	return nil
}

func gdemuPath(root string) string {
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), "GDEMU.INI") {
			return filepath.Join(root, e.Name())
		}
	}
	return filepath.Join(root, "GDEMU.INI")
}

func ReadGDEMU(root string) (*GDEMUSettings, error) {
	s := &GDEMUSettings{}
	b, err := os.ReadFile(gdemuPath(root))
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	s.Exists = true
	var other []string
	for _, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		k, v, ok := strings.Cut(line, "=")
		if ok {
			if f := s.field(strings.ToLower(strings.TrimSpace(k))); f != nil {
				*f = strings.TrimSpace(v)
				continue
			}
		}
		if strings.TrimSpace(line) != "" {
			other = append(other, line)
		}
	}
	s.Other = strings.Join(other, "\n")
	return s, nil
}

func checkRange(name, v string, lo, hi int, extra ...int) error {
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err == nil {
		for _, x := range extra {
			if n == x {
				return nil
			}
		}
		if n >= lo && n <= hi {
			return nil
		}
	}
	return fmt.Errorf("%s must be a number from %d to %d", name, lo, hi)
}

func SaveGDEMU(root string, s GDEMUSettings) error {
	errs := []error{
		checkRange("Lid open time", s.OpenTime, 150, 5000),
		checkRange("Detect time", s.DetectTime, 150, 5000),
		checkRange("Reset button folder", s.ResetGoto, 0, 9999),
		checkRange("Image checks", s.ImageTests, 0, 1),
		checkRange("High speed SD", s.HighSpeed, 0, 1),
		checkRange("Read speed limit", s.ReadLimit, 600, 1250, -1, 0),
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	var b strings.Builder
	for _, k := range gdemuKeys {
		if v := strings.TrimSpace(*s.field(k)); v != "" {
			fmt.Fprintf(&b, "%s = %s\r\n", k, v)
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(s.Other, "\r\n", "\n"), "\n") {
		k, _, ok := strings.Cut(line, "=")
		if ok && (&GDEMUSettings{}).field(strings.ToLower(strings.TrimSpace(k))) != nil {
			continue // a managed key typed into the extra box would be written twice
		}
		if strings.TrimSpace(line) != "" {
			b.WriteString(strings.TrimRight(line, " \t") + "\r\n")
		}
	}
	p := gdemuPath(root)
	if old, err := os.ReadFile(p); err == nil {
		os.MkdirAll(filepath.Join(root, backupDir), 0o755)
		os.WriteFile(filepath.Join(root, backupDir, "GDEMU.INI.bak"), old, 0o644)
	}
	return os.WriteFile(p, []byte(b.String()), 0o644)
}
