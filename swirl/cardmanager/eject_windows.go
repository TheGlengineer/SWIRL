//go:build windows

package main

// Safe eject (2.17): the same as Windows' own "Eject" on the drive, done from the window once every write
// has settled, so the owner never pulls a card mid write. Windows flushes and dismounts the volume and shows
// its usual "safe to remove" notice; a card reader that cannot eject still gets the flush and dismount.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func ejectDrive(root string) error {
	drive := strings.ToUpper(strings.TrimRight(filepath.VolumeName(root), `\`))
	if len(drive) != 2 || drive[1] != ':' {
		return errors.New("only a drive letter can be ejected")
	}
	if _, err := os.Stat(root); err != nil {
		return errors.New("the card is not in the reader")
	}
	// flush and dismount first, which is what makes the card safe; the shell verb then ejects the media
	if err := dismountVolume(drive); err != nil {
		return err
	}
	ps := `(New-Object -ComObject Shell.Application).NameSpace(17).ParseName(` + psQuote(drive+`\`) + `).InvokeVerb('Eject')`
	powershell(ps) // best effort: some readers have no eject, the dismount above already made it safe
	return nil
}

const (
	ioctlStorageMediaRemoval = 0x2D4804
	ioctlStorageEjectMedia   = 0x2D4808
)

func dismountVolume(drive string) error {
	h, err := openDev(`\\.\`+drive, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0)
	if err != nil {
		return errors.New("the card could not be opened for ejecting; close any window showing it and try again")
	}
	defer syscall.CloseHandle(h)
	syscall.FlushFileBuffers(h)
	if _, err := ioctl(h, fsctlLockVolume, nil, nil); err != nil {
		return errors.New("the card is in use by another program (an Explorer window or a file open on it); close it and try again")
	}
	ioctl(h, fsctlDismountVolume, nil, nil)
	ioctl(h, ioctlStorageMediaRemoval, []byte{0}, nil) // PREVENT_MEDIA_REMOVAL = FALSE
	ioctl(h, ioctlStorageEjectMedia, nil, nil)
	return nil
}
