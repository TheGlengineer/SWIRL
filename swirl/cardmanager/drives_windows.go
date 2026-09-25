//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

type Drive struct {
	Path      string `json:"path"`
	Label     string `json:"label"`
	Type      string `json:"type"` // removable, fixed, network, cd, ram
	Removable bool   `json:"removable"`
	HasMenu   bool   `json:"hasMenu"`
	Ready     bool   `json:"ready"`
	FS        string `json:"fs,omitempty"`
	Total     uint64 `json:"total,omitempty"`
	Free      uint64 `json:"free,omitempty"`
}

var (
	getDiskFreeSpaceEx   = syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW")
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	ole32                = syscall.NewLazyDLL("ole32.dll")
	getLogicalDrives     = kernel32.NewProc("GetLogicalDrives")
	getDriveType         = kernel32.NewProc("GetDriveTypeW")
	getVolumeInfo        = kernel32.NewProc("GetVolumeInformationW")
	setErrorMode         = kernel32.NewProc("SetErrorMode")
	shGetKnownFolderPath = shell32.NewProc("SHGetKnownFolderPath")
	coTaskMemFree        = ole32.NewProc("CoTaskMemFree")
)

var driveTypes = map[uintptr]string{2: "removable", 3: "fixed", 4: "network", 5: "cd", 6: "ram"}

// listDrives returns every drive Windows has a letter for: fixed and removable disks, card readers,
// network drives and optical drives (empty ones are listed as not ready).
func listDrives() []Drive {
	// an empty card reader or DVD drive must not pop up "There is no disk in the drive"
	setErrorMode.Call(0x0001 | 0x8000) // SEM_FAILCRITICALERRORS | SEM_NOOPENFILEERRORBOX
	var out []Drive
	mask, _, _ := getLogicalDrives.Call()
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		p, _ := syscall.UTF16PtrFromString(root)
		t, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(p)))
		typ, ok := driveTypes[t]
		if !ok {
			continue
		}
		var label, fsName [261]uint16
		r, _, _ := getVolumeInfo.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&label[0])), 261, 0, 0, 0, uintptr(unsafe.Pointer(&fsName[0])), 261)
		d := Drive{Path: root, Label: syscall.UTF16ToString(label[:]), Type: typ, Removable: t == 2, Ready: r != 0, FS: syscall.UTF16ToString(fsName[:])}
		if d.Ready {
			var free, total, totalFree uint64
			if ok, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree))); ok != 0 {
				d.Free, d.Total = free, total
			}
			if typ != "network" && typ != "cd" {
				if st, err := os.Stat(filepath.Join(root, "01")); err == nil && st.IsDir() {
					d.HasMenu = true
				}
			}
		}
		out = append(out, d)
	}
	return out
}

type knownGUID struct {
	d1     uint32
	d2, d3 uint16
	d4     [8]byte
}

func knownFolder(g knownGUID) string {
	var out *uint16
	r, _, _ := shGetKnownFolderPath.Call(uintptr(unsafe.Pointer(&g)), 0, 0, uintptr(unsafe.Pointer(&out)))
	if r != 0 || out == nil {
		return ""
	}
	defer coTaskMemFree.Call(uintptr(unsafe.Pointer(out)))
	return syscall.UTF16ToString((*[1 << 15]uint16)(unsafe.Pointer(out))[:])
}

// places are the folders people keep downloads in, found the way Explorer finds them (so a Desktop or
// Documents folder moved into OneDrive is still right).
func places() []Place {
	list := []struct {
		name string
		g    knownGUID
	}{
		{"Desktop", knownGUID{0xB4BFCC3A, 0xDB2C, 0x424C, [8]byte{0xB0, 0x29, 0x7F, 0xE9, 0x9A, 0x87, 0xC6, 0x41}}},
		{"Downloads", knownGUID{0x374DE290, 0x123F, 0x4565, [8]byte{0x91, 0x64, 0x39, 0xC4, 0x92, 0x5E, 0x46, 0x7B}}},
		{"Documents", knownGUID{0xFDD39AD0, 0x238F, 0x46AF, [8]byte{0xAD, 0xB4, 0x6C, 0x85, 0x48, 0x03, 0x69, 0xC7}}},
		{"Home", knownGUID{0x5E6C858F, 0x0E22, 0x4760, [8]byte{0x9A, 0xFE, 0xEA, 0x33, 0x17, 0xB6, 0x71, 0x73}}},
	}
	var out []Place
	for _, k := range list {
		if p := knownFolder(k.g); p != "" {
			out = append(out, Place{Name: k.name, Path: p})
		}
	}
	if od := os.Getenv("OneDrive"); od != "" {
		out = append(out, Place{Name: "OneDrive", Path: od})
	}
	return out
}

// hiddenFile reports files Explorer hides (hidden or system attribute).
func hiddenFile(info os.FileInfo) bool {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return d.FileAttributes&(syscall.FILE_ATTRIBUTE_HIDDEN|syscall.FILE_ATTRIBUTE_SYSTEM) != 0
	}
	return false
}

func openBrowser(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

func freeSpace(root string) (uint64, bool) {
	p, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return 0, false
	}
	var free uint64
	r, _, _ := getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&free)), 0, 0)
	return free, r != 0
}

var getDiskFreeSpace = kernel32.NewProc("GetDiskFreeSpaceW")

// volumeInfo reports the file system name, cluster size and space of the card.
func volumeInfo(root string) (fs string, cluster int, total, free uint64) {
	r := filepath.VolumeName(root) + `\`
	p, err := syscall.UTF16PtrFromString(r)
	if err != nil {
		return
	}
	var name [64]uint16
	getVolumeInfo.Call(uintptr(unsafe.Pointer(p)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&name[0])), 64)
	fs = syscall.UTF16ToString(name[:])
	var spc, bps, freeC, totalC uint32
	if ok, _, _ := getDiskFreeSpace.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&spc)), uintptr(unsafe.Pointer(&bps)), uintptr(unsafe.Pointer(&freeC)), uintptr(unsafe.Pointer(&totalC))); ok != 0 {
		cluster = int(spc * bps)
		total = uint64(totalC) * uint64(cluster)
		free = uint64(freeC) * uint64(cluster)
	}
	return
}
