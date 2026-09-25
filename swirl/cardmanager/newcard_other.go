//go:build !windows && !darwin

package main

import "errors"

func diskInfo(root string) (*DiskInfo, error) {
	return nil, errors.New("formatting a card is only available on Windows")
}

func formatCard(root string, disk int, report func(float64, string)) (string, error) {
	return "", errors.New("formatting a card is only available on Windows")
}

func runFormatHelper(root string, disk int, status string) int { return 1 }
