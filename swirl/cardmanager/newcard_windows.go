//go:build windows

package main

// Windows raw disk access for "New card": find the physical card behind a drive letter,
// lock it, write a fresh MBR + FAT32, and get a drive letter back.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	ioctlStorageGetDeviceNumber = 0x2D1080
	ioctlStorageQueryProperty   = 0x2D1400
	ioctlDiskGetLengthInfo      = 0x7405C
	ioctlDiskGetGeometryEx      = 0x700A0
	ioctlDiskUpdateProperties   = 0x70140
	fsctlLockVolume             = 0x90018
	fsctlDismountVolume         = 0x90020
	fileFlagNoBuffering         = 0x20000000
	fileFlagWriteThrough        = 0x80000000
)

var (
	shell32                 = syscall.NewLazyDLL("shell32.dll")
	shellExecuteEx          = shell32.NewProc("ShellExecuteExW")
	findFirstVolume         = kernel32.NewProc("FindFirstVolumeW")
	findNextVolume          = kernel32.NewProc("FindNextVolumeW")
	findVolumeClose         = kernel32.NewProc("FindVolumeClose")
	getVolumePathNames      = kernel32.NewProc("GetVolumePathNamesForVolumeNameW")
	setVolumeMountPoint     = kernel32.NewProc("SetVolumeMountPointW")
	getExitCodeProcess      = kernel32.NewProc("GetExitCodeProcess")
	waitForSingleObject     = kernel32.NewProc("WaitForSingleObject")
	flushFileBuffers        = kernel32.NewProc("FlushFileBuffers")
	getFileSystemInfoVolume = kernel32.NewProc("GetVolumeInformationW")
)

var busNames = map[uint32]string{1: "SCSI", 2: "ATAPI", 3: "ATA", 4: "FireWire", 5: "SSA", 6: "Fibre", 7: "USB", 8: "RAID", 9: "iSCSI", 10: "SAS", 11: "SATA", 12: "SD", 13: "MMC", 14: "Virtual", 15: "Virtual file", 16: "Storage Spaces", 17: "NVMe"}

func openDev(path string, access uint32, flags uint32) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	return syscall.CreateFile(p, access, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, flags, 0)
}

func ioctl(h syscall.Handle, code uint32, in []byte, out []byte) (uint32, error) {
	var n uint32
	var ip, op *byte
	if len(in) > 0 {
		ip = &in[0]
	}
	if len(out) > 0 {
		op = &out[0]
	}
	err := syscall.DeviceIoControl(h, code, ip, uint32(len(in)), op, uint32(len(out)), &n, nil)
	return n, err
}

// deviceNumber returns the physical disk number behind a volume path like \\.\G: or \\?\Volume{...}.
func deviceNumber(volPath string) (int, error) {
	h, err := openDev(volPath, 0, 0)
	if err != nil {
		return -1, err
	}
	defer syscall.CloseHandle(h)
	out := make([]byte, 12)
	if _, err := ioctl(h, ioctlStorageGetDeviceNumber, nil, out); err != nil {
		return -1, err
	}
	return int(*(*uint32)(unsafe.Pointer(&out[4]))), nil
}

func systemDisk() int {
	sd := os.Getenv("SystemDrive")
	if sd == "" {
		sd = "C:"
	}
	n, err := deviceNumber(`\\.\` + strings.TrimSuffix(sd, `\`))
	if err != nil {
		return -1
	}
	return n
}

// volumesOnDisk lists \\?\Volume{...}\ names that live on a physical disk.
func volumesOnDisk(disk int) []string {
	var out []string
	buf := make([]uint16, 300)
	h, _, _ := findFirstVolume.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if syscall.Handle(h) == syscall.InvalidHandle {
		return nil
	}
	defer findVolumeClose.Call(h)
	for {
		name := syscall.UTF16ToString(buf)
		if n, err := deviceNumber(strings.TrimSuffix(name, `\`)); err == nil && n == disk {
			out = append(out, name)
		}
		r, _, _ := findNextVolume.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if r == 0 {
			break
		}
	}
	return out
}

func volumeMountPaths(vol string) []string {
	p, _ := syscall.UTF16PtrFromString(vol)
	buf := make([]uint16, 1024)
	var n uint32
	r, _, _ := getVolumePathNames.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] == 0 {
			if i == start {
				break
			}
			out = append(out, syscall.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	return out
}

func fsName(root string) string {
	p, _ := syscall.UTF16PtrFromString(root)
	var fs [64]uint16
	r, _, _ := getFileSystemInfoVolume.Call(uintptr(unsafe.Pointer(p)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&fs[0])), 64)
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(fs[:])
}

func cstrAt(b []byte, off uint32) string {
	if off == 0 || int(off) >= len(b) {
		return ""
	}
	end := int(off)
	for end < len(b) && b[end] != 0 {
		end++
	}
	return strings.TrimSpace(string(b[off:end]))
}

func diskInfo(root string) (*DiskInfo, error) {
	letter := driveLetter(root)
	if letter == "" {
		return nil, errors.New("pick the card by its drive letter, for example G:")
	}
	info := &DiskInfo{Root: letter + `:\`, Confirm: letter}
	disk, err := deviceNumber(`\\.\` + letter + ":")
	if err != nil {
		return nil, fmt.Errorf("%s: is not a disk that can be formatted (%v)", letter, err)
	}
	info.Disk = disk
	h, err := openDev(fmt.Sprintf(`\\.\PhysicalDrive%d`, disk), 0, 0)
	if err != nil {
		return nil, err
	}
	defer syscall.CloseHandle(h)
	q := make([]byte, 12) // StorageDeviceProperty, PropertyStandardQuery
	desc := make([]byte, 1024)
	var bus uint32
	if _, err := ioctl(h, ioctlStorageQueryProperty, q, desc); err == nil {
		info.Removable = desc[10] != 0
		vendor := cstrAt(desc, *(*uint32)(unsafe.Pointer(&desc[12])))
		product := cstrAt(desc, *(*uint32)(unsafe.Pointer(&desc[16])))
		info.Model = strings.TrimSpace(vendor + " " + product)
		bus = *(*uint32)(unsafe.Pointer(&desc[28]))
		info.Bus = busNames[bus]
	}
	geo := make([]byte, 256)
	if _, err := ioctl(h, ioctlDiskGetGeometryEx, nil, geo); err == nil {
		info.SizeBytes = *(*uint64)(unsafe.Pointer(&geo[24]))
		if bps := *(*uint32)(unsafe.Pointer(&geo[20])); bps != secSize {
			info.Reason = fmt.Sprintf("this card uses %d byte sectors; only 512 is supported", bps)
		}
	}
	for _, v := range volumesOnDisk(disk) {
		for _, p := range volumeMountPaths(v) {
			if len(p) <= 3 {
				info.Letters = append(info.Letters, p)
			}
		}
	}
	if info.Model == "" {
		info.Model = "Unknown card"
	}
	switch {
	case info.Reason != "":
	case disk == systemDisk():
		info.Reason = "this drive holds Windows and can never be formatted here"
	case bus == 3 || bus == 8 || bus == 10 || bus == 11 || bus == 16 || bus == 17:
		info.Reason = fmt.Sprintf("this is an internal %s drive, not an SD card", info.Bus)
	case !(bus == 7 || bus == 12 || bus == 13 || info.Removable):
		info.Reason = "this does not look like an SD card or USB card reader"
	case info.SizeBytes == 0:
		info.Reason = "could not read the card size; is a card in the reader?"
	case info.SizeBytes > 2<<40:
		info.Reason = "cards larger than 2 TB are not supported"
	case info.SizeBytes < 128<<20:
		info.Reason = "the card is too small"
	default:
		info.OK = true
	}
	return info, nil
}

func isElevated() bool {
	p, err := syscall.GetCurrentProcess()
	if err != nil {
		return false
	}
	var t syscall.Token
	if syscall.OpenProcessToken(p, syscall.TOKEN_QUERY, &t) != nil {
		return false
	}
	defer t.Close()
	var elev, n uint32
	if err := syscall.GetTokenInformation(t, 20 /* TokenElevation */, (*byte)(unsafe.Pointer(&elev)), 4, &n); err != nil {
		return false
	}
	return elev != 0
}

// alignedDisk writes to a raw disk handle opened without buffering; buffers must be sector aligned.
type alignedDisk struct {
	h   syscall.Handle
	raw []byte
}

func (a *alignedDisk) WriteAt(p []byte, off int64) (int, error) {
	need := len(p) + 4096
	if len(a.raw) < need {
		a.raw = make([]byte, need)
	}
	shift := int((4096 - uintptr(unsafe.Pointer(&a.raw[0]))%4096) % 4096)
	buf := a.raw[shift : shift+len(p)]
	copy(buf, p)
	var ov syscall.Overlapped
	ov.Offset = uint32(off)
	ov.OffsetHigh = uint32(off >> 32)
	var n uint32
	err := syscall.WriteFile(a.h, buf, &n, &ov)
	if err == nil && int(n) != len(p) {
		err = errors.New("short write")
	}
	return int(n), err
}

// doFormat does the destructive part. It must run elevated.
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
	report(0, "Unmounting the card")
	var locked []syscall.Handle
	defer func() {
		for _, h := range locked {
			syscall.CloseHandle(h)
		}
	}()
	for _, v := range volumesOnDisk(disk) {
		h, err := openDev(strings.TrimSuffix(v, `\`), syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0)
		if err != nil {
			continue
		}
		ok := false
		for i := 0; i < 10 && !ok; i++ {
			if _, err := ioctl(h, fsctlLockVolume, nil, nil); err == nil {
				ok = true
			} else {
				time.Sleep(500 * time.Millisecond)
			}
		}
		// dismount even if the lock failed (for example an Explorer window is open on the card)
		ioctl(h, fsctlDismountVolume, nil, nil)
		locked = append(locked, h)
	}
	h, err := openDev(fmt.Sprintf(`\\.\PhysicalDrive%d`, disk), syscall.GENERIC_READ|syscall.GENERIC_WRITE, fileFlagNoBuffering|fileFlagWriteThrough)
	if err != nil {
		if errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
			return "", errors.New("Windows refused access to the card; is the card's lock switch on?")
		}
		return "", err
	}
	out := make([]byte, 8)
	if _, err := ioctl(h, ioctlDiskGetLengthInfo, nil, out); err != nil {
		syscall.CloseHandle(h)
		return "", err
	}
	size := *(*uint64)(unsafe.Pointer(&out[0]))
	l, err := planFAT32(size/secSize, "SWIRL")
	if err != nil {
		syscall.CloseHandle(h)
		return "", err
	}
	report(0.02, "Writing FAT32")
	err = writeFreshCard(&alignedDisk{h: h}, l, func(done, total uint64) {
		report(0.02+0.9*float64(done)/float64(total), "")
	})
	if err != nil {
		syscall.CloseHandle(h)
		if errors.Is(err, syscall.Errno(19)) { // ERROR_WRITE_PROTECT
			return "", errors.New("the card is write protected; slide its lock switch up and try again")
		}
		return "", err
	}
	flushFileBuffers.Call(uintptr(h))
	ioctl(h, ioctlDiskUpdateProperties, nil, nil)
	syscall.CloseHandle(h)
	for _, lh := range locked {
		syscall.CloseHandle(lh)
	}
	locked = nil

	report(0.95, "Waiting for Windows to mount the card")
	want := info.Root
	deadline := time.Now().Add(40 * time.Second)
	assigned := false
	for time.Now().Before(deadline) {
		time.Sleep(time.Second)
		for _, v := range volumesOnDisk(disk) {
			paths := volumeMountPaths(v)
			for _, p := range paths {
				if len(p) <= 3 && fsName(p) == "FAT32" {
					report(1, "")
					return p, nil
				}
			}
			if len(paths) == 0 && !assigned && time.Until(deadline) < 34*time.Second {
				// Windows did not give it a letter; give it back the one it had
				mp, _ := syscall.UTF16PtrFromString(want)
				vp, _ := syscall.UTF16PtrFromString(v)
				setVolumeMountPoint.Call(uintptr(unsafe.Pointer(mp)), uintptr(unsafe.Pointer(vp)))
				assigned = true
			}
		}
	}
	return "", errors.New("the card was formatted but Windows did not mount it; unplug the card, plug it back in, then use Install SWIRL")
}

// runFormatHelper is the elevated child process started by formatCard.
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

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	hIcon        uintptr
	hProcess     syscall.Handle
}

// formatCard erases and formats the card. If this app is not running as administrator,
// Windows asks for permission (UAC) and a helper copy of the app does the formatting.
func formatCard(root string, disk int, report func(float64, string)) (string, error) {
	if isElevated() {
		return doFormat(root, disk, report)
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	status := filepath.Join(os.TempDir(), fmt.Sprintf("swirl_format_%d.json", time.Now().UnixNano()))
	defer os.Remove(status)
	args := fmt.Sprintf(`-format-disk %s: -disk %d -status "%s"`, driveLetter(root), disk, status)
	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	params, _ := syscall.UTF16PtrFromString(args)
	sei := shellExecuteInfo{fMask: 0x40 | 0x100, lpVerb: verb, lpFile: file, lpParameters: params, nShow: 0}
	sei.cbSize = uint32(unsafe.Sizeof(sei))
	report(0, "Waiting for you to allow administrator access")
	r, _, e := shellExecuteEx.Call(uintptr(unsafe.Pointer(&sei)))
	if r == 0 {
		if errno, ok := e.(syscall.Errno); ok && errno == 1223 {
			return "", errors.New("administrator access was declined, so the card was not touched")
		}
		return "", fmt.Errorf("could not start the formatter: %v", e)
	}
	defer syscall.CloseHandle(sei.hProcess)
	for {
		w, _, _ := waitForSingleObject.Call(uintptr(sei.hProcess), 400)
		var s fmtStatus
		if b, err := os.ReadFile(status); err == nil && json.Unmarshal(b, &s) == nil {
			report(s.Pct, s.Msg)
			if s.Done {
				if s.Error != "" {
					return "", errors.New(s.Error)
				}
				return s.Root, nil
			}
		}
		if w == 0 { // process ended
			var code uint32
			getExitCodeProcess.Call(uintptr(sei.hProcess), uintptr(unsafe.Pointer(&code)))
			if b, err := os.ReadFile(status); err == nil && json.Unmarshal(b, &s) == nil && s.Done {
				if s.Error != "" {
					return "", errors.New(s.Error)
				}
				return s.Root, nil
			}
			return "", fmt.Errorf("the formatter stopped unexpectedly (code %d)", code)
		}
	}
}
