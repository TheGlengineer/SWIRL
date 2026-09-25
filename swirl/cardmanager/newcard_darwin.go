//go:build darwin

package main

// Formatting a card on macOS. The card is identified with diskutil and checked against the same rules as
// on Windows (never the startup disk, never an internal drive or disk image). Writing the raw disk needs
// administrator rights, so macOS asks for the password and a helper copy of the app does the formatting
// with SWIRL's own FAT32 writer (the same code as on Windows: MBR, 32 KB clusters, label SWIRL).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// bootDisks lists the disk numbers that hold the running macOS: the container and its physical store.
func bootDisks() map[int]bool {
	out := map[int]bool{}
	for _, p := range []string{"/", "/System/Volumes/Data"} {
		d, err := diskutilInfo(p)
		if err != nil {
			continue
		}
		if n := macDiskNumber(d.str("ParentWholeDisk")); n >= 0 {
			out[n] = true
		}
		if stores, ok := d["APFSPhysicalStores"].([]any); ok {
			for _, s := range stores {
				if m, ok := s.(map[string]any); ok {
					if n := macDiskNumber(plistDict(m).str("APFSPhysicalStore")); n >= 0 {
						out[n] = true
					}
				}
			}
		}
	}
	return out
}

// mountsOnDisk lists the mounted volumes of a whole disk.
func mountsOnDisk(disk int) []string {
	var out []string
	entries, _ := os.ReadDir("/Volumes")
	for _, e := range entries {
		m := filepath.Join("/Volumes", e.Name())
		if st, err := os.Lstat(m); err != nil || st.Mode()&os.ModeSymlink != 0 || !isMount(m) {
			continue
		}
		d, err := diskutilInfo(m)
		if err == nil && macDiskNumber(d.str("ParentWholeDisk")) == disk {
			out = append(out, m)
		}
	}
	return out
}

func diskInfo(root string) (*DiskInfo, error) {
	root = strings.TrimSpace(root)
	if root == "" || !strings.HasPrefix(filepath.Clean(root), "/Volumes/") {
		return nil, errors.New("pick the card from the list; it appears under /Volumes once it is in the reader")
	}
	vol, err := diskutilInfo(root)
	if err != nil {
		return nil, fmt.Errorf("%s is not a disk macOS can describe (%v)", root, err)
	}
	whole := vol.str("ParentWholeDisk")
	if whole == "" {
		return nil, fmt.Errorf("%s is not on a physical disk", root)
	}
	wd, err := diskutilInfo(whole)
	if err != nil {
		return nil, err
	}
	name := vol.str("VolumeName")
	if name == "" {
		name = filepath.Base(root)
	}
	info := &DiskInfo{Root: filepath.Clean(root), Name: name, Confirm: macConfirmWord(name)}
	judgeMacDisk(info, wd, bootDisks())
	if info.Disk >= 0 {
		info.Letters = mountsOnDisk(info.Disk)
	}
	if len(info.Letters) == 0 {
		info.Letters = []string{info.Root}
	}
	return info, nil
}

func diskutil(args ...string) error {
	out, err := exec.Command("/usr/sbin/diskutil", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("diskutil %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// doFormat does the destructive part. It must run as root.
func doFormat(root string, disk int, report func(float64, string)) (string, error) {
	info, err := diskInfo(root)
	if err != nil {
		return "", err
	}
	if !info.OK {
		return "", errors.New(info.Reason)
	}
	if info.Disk != disk {
		return "", errors.New("the drive changed since you picked it; pick the card again")
	}
	dev := fmt.Sprintf("/dev/disk%d", disk)
	report(0, "Unmounting the card")
	if err := diskutil("unmountDisk", "force", dev); err != nil {
		return "", fmt.Errorf("macOS could not unmount the card; close any Finder windows or apps using it and try again (%v)", err)
	}
	f, err := os.OpenFile(fmt.Sprintf("/dev/rdisk%d", disk), os.O_RDWR, 0)
	if err != nil {
		diskutil("mountDisk", dev)
		if errors.Is(err, syscall.EROFS) || errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
			return "", errors.New("the card is write protected; slide its lock switch up and try again")
		}
		return "", err
	}
	l, err := planFAT32(info.SizeBytes/secSize, "SWIRL")
	if err != nil {
		f.Close()
		return "", err
	}
	report(0.02, "Writing FAT32")
	err = writeFreshCard(f, l, func(done, total uint64) {
		report(0.02+0.9*float64(done)/float64(total), "")
	})
	if err == nil {
		err = f.Sync()
	}
	f.Close()
	if err != nil {
		diskutil("mountDisk", dev)
		if errors.Is(err, syscall.EROFS) || errors.Is(err, syscall.EPERM) {
			return "", errors.New("the card is write protected; slide its lock switch up and try again")
		}
		return "", err
	}

	report(0.95, "Waiting for macOS to mount the card")
	deadline := time.Now().Add(40 * time.Second)
	tried := false
	for time.Now().Before(deadline) {
		if !tried {
			diskutil("mountDisk", dev)
			tried = true
		}
		time.Sleep(time.Second)
		for _, m := range mountsOnDisk(disk) {
			if d, err := diskutilInfo(m); err == nil && fsLabel(d) == "FAT32" {
				// keep Spotlight from writing its index onto the card
				os.WriteFile(filepath.Join(m, ".metadata_never_index"), nil, 0o644)
				exec.Command("/usr/bin/mdutil", "-i", "off", m).Run()
				report(1, "")
				return m, nil
			}
		}
		if time.Until(deadline) < 30*time.Second {
			tried = false
		}
	}
	return "", errors.New("the card was formatted but macOS did not mount it; take the card out, put it back in, then use Install SWIRL")
}

// runFormatHelper is the child process started with administrator rights by formatCard.
func runFormatHelper(root string, disk int, status string) int {
	last := time.Time{}
	var cur fmtStatus
	root, err := doFormat(root, disk, func(p float64, msg string) {
		cur.Pct = p
		if msg != "" {
			cur.Msg = msg
		}
		if time.Since(last) > 300*time.Millisecond || msg != "" {
			writeStatus(status, cur)
			last = time.Now()
		}
	})
	cur.Done = true
	if err != nil {
		cur.Error = err.Error()
		writeStatus(status, cur)
		return 1
	}
	cur.Root, cur.Pct = root, 1
	writeStatus(status, cur)
	return 0
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// formatCard erases and formats the card. macOS asks for the administrator password, then a helper copy
// of this app does the work and reports progress through a small status file.
func formatCard(root string, disk int, report func(float64, string)) (string, error) {
	if os.Geteuid() == 0 {
		return doFormat(root, disk, report)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	status := filepath.Join(os.TempDir(), fmt.Sprintf("swirl_format_%d.json", time.Now().UnixNano()))
	defer os.Remove(status)
	defer os.Remove(status + ".tmp")
	cmd := fmt.Sprintf("%s -format-disk %s -disk %d -status %s >/dev/null 2>&1", shellQuote(exe), shellQuote(root), disk, shellQuote(status))
	script := "do shell script " + appleScriptQuote(cmd) + " with administrator privileges with prompt " +
		appleScriptQuote("SWIRL Card Manager needs your password to erase and format the SD card.")
	report(0, "Waiting for your password")
	osa := exec.Command("/usr/bin/osascript", "-e", script)
	var errOut strings.Builder
	osa.Stderr = &errOut
	if err := osa.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- osa.Wait() }()
	readStatus := func() (fmtStatus, bool) {
		var s fmtStatus
		b, err := os.ReadFile(status)
		if err != nil || json.Unmarshal(b, &s) != nil {
			return s, false
		}
		return s, true
	}
	for {
		select {
		case err := <-done:
			if s, ok := readStatus(); ok && s.Done {
				if s.Error != "" {
					return "", errors.New(s.Error)
				}
				return s.Root, nil
			}
			if err != nil && strings.Contains(errOut.String(), "-128") {
				return "", errors.New("the password was not entered, so the card was not touched")
			}
			if err != nil {
				return "", fmt.Errorf("the formatter could not run: %s", strings.TrimSpace(errOut.String()))
			}
			return "", errors.New("the formatter stopped unexpectedly")
		case <-time.After(400 * time.Millisecond):
			if s, ok := readStatus(); ok {
				report(s.Pct, s.Msg)
			}
		}
	}
}
