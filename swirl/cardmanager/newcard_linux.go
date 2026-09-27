//go:build linux

package main

// Formatting a card on Linux. The card is found through /proc/self/mountinfo and
// sysfs, and checked against the same safety rules as on Windows and macOS (never
// the disk that holds /, never an internal drive that is not on USB). Writing the
// raw disk needs root, so SWIRL re-execs itself through pkexec or sudo and a helper
// copy does the formatting with SWIRL's own FAT32 writer (the same code as the other
// platforms: MBR, 32 KB clusters, label SWIRL).

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func sysfsRead(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// linuxReadDevice builds the sysfs view of a whole block device.
func linuxReadDevice(name string) linuxBlockDev {
	base := filepath.Join("/sys/block", name)
	d := linuxBlockDev{Name: name}
	d.Model = sysfsRead(filepath.Join(base, "device", "model"))
	d.Removable = parseSysfsBool(sysfsRead(filepath.Join(base, "removable")))
	d.SizeBytes = parseSysfsSectors(sysfsRead(filepath.Join(base, "size")))
	if s := sysfsRead(filepath.Join(base, "queue", "logical_block_size")); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			d.SecSize = n
		}
	}
	if link, err := filepath.EvalSymlinks(filepath.Join(base, "device")); err == nil {
		d.USB = strings.Contains(link, "/usb")
	}
	return d
}

// wholeDeviceForMajorMinor resolves a device major:minor to its whole block device name.
func wholeDeviceForMajorMinor(major, minor int) string {
	link, err := os.Readlink(fmt.Sprintf("/sys/dev/block/%d:%d", major, minor))
	if err != nil {
		return ""
	}
	whole, _ := parseSysfsBlockLink(link)
	return whole
}

// linuxDiskForRoot resolves a mount point to its whole block device and sysfs view.
func linuxDiskForRoot(root string) (linuxBlockDev, string, error) {
	entries, err := readMountinfo()
	if err != nil {
		return linuxBlockDev{}, "", fmt.Errorf("cannot read /proc/self/mountinfo: %v", err)
	}
	e, ok := mountEntryFor(entries, root)
	if !ok {
		return linuxBlockDev{}, "", fmt.Errorf("%s is not on a mounted filesystem", root)
	}
	whole := wholeDeviceForMajorMinor(e.Major, e.Minor)
	if whole == "" {
		return linuxBlockDev{}, "", fmt.Errorf("%s is not on a physical disk", root)
	}
	return linuxReadDevice(whole), whole, nil
}

// rootDeviceName is the whole block device that holds the running system's root filesystem.
func rootDeviceName() string {
	entries, err := readMountinfo()
	if err != nil {
		return ""
	}
	if e, ok := mountEntryFor(entries, "/"); ok {
		return wholeDeviceForMajorMinor(e.Major, e.Minor)
	}
	return ""
}

// mountsOnDevice lists every mount point on a whole block device.
func mountsOnDevice(whole string) []string {
	entries, _ := readMountinfo()
	var out []string
	for _, e := range entries {
		if wholeDeviceForMajorMinor(e.Major, e.Minor) == whole {
			out = append(out, e.MountPoint)
		}
	}
	return out
}

// mountPointFor picks where the freshly formatted card is mounted. The mount must
// live in a directory the owning user can reach; /media/<user> is the udisks
// convention, with /run/media/<user> as a fallback on systems that use it.
func mountPointFor(whole, user string) string {
	if user == "" {
		user = os.Getenv("SUDO_USER")
	}
	if user == "" {
		user = os.Getenv("USER")
	}
	if user != "" {
		for _, base := range []string{"/media", "/run/media"} {
			mnt := filepath.Join(base, user, "SWIRL")
			if os.MkdirAll(filepath.Dir(mnt), 0o755) == nil {
				return mnt
			}
		}
		return filepath.Join("/media", user, "SWIRL")
	}
	return filepath.Join("/mnt", "SWIRL")
}

// mountOpts returns the vfat mount options that give the owning user write access.
func mountOpts(uid, gid int) string {
	if uid >= 0 && gid >= 0 {
		return fmt.Sprintf("uid=%d,gid=%d,umask=002", uid, gid)
	}
	return ""
}

func diskInfo(root string) (*DiskInfo, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("pick the card from the list; it appears under /media or /run/media once it is in the reader")
	}
	dev, whole, err := linuxDiskForRoot(root)
	if err != nil {
		return nil, fmt.Errorf("%s is not a disk Linux can describe (%v)", root, err)
	}
	info := &DiskInfo{
		Root:    filepath.Clean(root),
		Name:    filepath.Base(root),
		Confirm: linuxConfirmWord(whole),
		Letters: []string{filepath.Clean(root)},
	}
	judgeLinuxDisk(info, dev, rootDeviceName())
	if info.Disk >= 0 {
		info.Letters = mountsOnDevice(whole)
	}
	if len(info.Letters) == 0 {
		info.Letters = []string{info.Root}
	}
	return info, nil
}

// doFormat does the destructive part. It must run as root.
func doFormat(root string, disk int, uid, gid int, user string, report func(float64, string)) (string, error) {
	dev, whole, err := linuxDiskForRoot(root)
	if err != nil {
		return "", err
	}
	info := &DiskInfo{Root: filepath.Clean(root), Name: filepath.Base(root), Confirm: linuxConfirmWord(whole)}
	judgeLinuxDisk(info, dev, rootDeviceName())
	if !info.OK {
		return "", errors.New(info.Reason)
	}
	if info.Disk != disk {
		return "", errors.New("the drive changed since you picked it; pick the card again")
	}
	devPath := filepath.Join("/dev", whole)
	report(0, "Unmounting the card")
	for _, mp := range mountsOnDevice(whole) {
		if mp == "/" {
			continue
		}
		syscall.Unmount(mp, syscall.MNT_DETACH)
	}
	f, err := os.OpenFile(devPath, os.O_RDWR|os.O_SYNC, 0)
	switch {
	case err == nil:
		l, perr := planFAT32(dev.SizeBytes/secSize, "SWIRL")
		if perr != nil {
			f.Close()
			return "", perr
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
			if errors.Is(err, syscall.EROFS) {
				return "", errors.New("the card is write protected; take it out, slide its lock switch up, away from LOCK, and put it back")
			}
			return "", fmt.Errorf("writing the card failed: %v", err)
		}
	case errors.Is(err, syscall.EROFS):
		return "", errors.New("the card is write protected; take it out, slide its lock switch up, away from LOCK, and put it back")
	case errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES):
		return "", errors.New("Linux refused access to the card; formatting needs root (the password prompt grants it)")
	default:
		return "", fmt.Errorf("the card could not be opened for formatting: %v", err)
	}

	report(0.95, "Waiting for Linux to mount the card")
	exec.Command("partprobe", devPath).Run()
	exec.Command("blockdev", "--rereadpt", devPath).Run()
	part := filepath.Join("/dev", partDev(whole))
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		if _, err := os.Stat(part); err != nil {
			continue
		}
		mnt := mountPointFor(whole, user)
		os.MkdirAll(mnt, 0o755)
		if syscall.Mount(part, mnt, "vfat", 0, mountOpts(uid, gid)) == nil {
			report(1, "")
			return mnt, nil
		}
	}
	return "", errors.New("the card was formatted but Linux did not mount it; unplug the card, plug it back in, then use Install SWIRL")
}

// runFormatHelper is the child process started with root rights by formatCard.
func runFormatHelper(root string, disk int, status string, uid, gid int, user string) int {
	last := time.Time{}
	var cur fmtStatus
	root, err := doFormat(root, disk, uid, gid, user, func(p float64, msg string) {
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

// elevatableExe returns a copy of this binary on a normal filesystem that root can
// read. The AppImage runtime keeps the real binary on a FUSE mount under
// /tmp/.mount_*, and FUSE denies access to anyone but the mounting user (root
// included), so pkexec/sudo would fail with "Permission denied". Copying the binary
// to a world-readable path in the data dir sidesteps that.
func elevatableExe() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(filepath.Clean(exe), filepath.Join(os.TempDir(), ".mount_")) {
		return exe, nil
	}
	dst := filepath.Join(filepath.Dir(dbDir()), "swirl-format-helper")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return "", err
	}
	dstExe := filepath.Join(dst, "SWIRL-Card-Manager")
	if same, _ := sameFile(exe, dstExe); same {
		return dstExe, nil
	}
	if err := copyFile(exe, dstExe); err != nil {
		return "", err
	}
	if err := os.Chmod(dstExe, 0o755); err != nil {
		return "", err
	}
	return dstExe, nil
}

// sameFile reports whether two paths are the same file (size and mtime agree).
func sameFile(a, b string) (bool, error) {
	sa, err := os.Stat(a)
	if err != nil {
		return false, err
	}
	sb, err := os.Stat(b)
	if err != nil {
		return false, err
	}
	return sa.Size() == sb.Size() && sa.ModTime().Equal(sb.ModTime()), nil
}

// formatCard erases and formats the card. If this app is not running as root, Linux
// asks for the password (pkexec, or sudo when there is no PolicyKit agent) and a
// helper copy of the app does the work, reporting progress through a status file.
func formatCard(root string, disk int, report func(float64, string)) (string, error) {
	if os.Geteuid() == 0 {
		return doFormat(root, disk, os.Getuid(), os.Getgid(), os.Getenv("USER"), report)
	}
	exe, err := elevatableExe()
	if err != nil {
		return "", err
	}
	status := filepath.Join(os.TempDir(), fmt.Sprintf("swirl_format_%d.json", time.Now().UnixNano()))
	defer os.Remove(status)
	defer os.Remove(status + ".tmp")
	uid, gid, user := os.Getuid(), os.Getgid(), os.Getenv("USER")
	if os.Getenv("USER") == "" {
		user = os.Getenv("LOGNAME")
	}
	args := []string{"-format-disk", root, "-disk", strconv.Itoa(disk), "-status", status,
		"-format-uid", strconv.Itoa(uid), "-format-gid", strconv.Itoa(gid), "-format-user", user}
	var cmd *exec.Cmd
	switch {
	case lookPath("pkexec") != "":
		cmd = exec.Command("pkexec", append([]string{exe}, args...)...)
	case lookPath("sudo") != "":
		cmd = exec.Command("sudo", append([]string{exe}, args...)...)
	default:
		return "", errors.New("formatting a card needs root. Install pkexec or sudo, or run SWIRL as root.")
	}
	report(0, "Waiting for your password")
	var errOut strings.Builder
	cmd.Stderr = &errOut
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("the formatter could not start: %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
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
			if err != nil && strings.Contains(errOut.String(), "NOT ALLOWED") {
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

func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}
