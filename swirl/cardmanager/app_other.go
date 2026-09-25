//go:build !windows && !darwin

package main

import "errors"

var windowKind = "browser"

func getAppInfo() AppInfo {
	return AppInfo{Version: version, Platform: "other", Window: windowKind, DataBytes: dirBytes(appDataDir())}
}

func openWindow(url string) { openBrowser(url) }

func InstallApp(desktop bool) error {
	return errors.New("installing is only available on Windows")
}

func SwitchToInstalled() error { return errors.New("installing is only available on Windows") }

func Uninstall(quiet bool) int { return 1 }

func waitForPID(pid int) {}

func startUninstall() error { return errors.New("installing is only available on Windows") }

const canSelfUpdate = false

func handOverToUpdate(exe string) error {
	return errors.New("updating from inside the app is only available on Windows")
}

func applyUpdate() int { return 1 }
