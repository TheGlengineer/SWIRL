package main

// cardFS is the set of file operations Card Manager uses when it writes to an SD card. The real one works
// on os; tests swap in a faulty one to check that a failure at any step leaves the card usable.

import (
	"io"
	"os"
	"runtime"
)

type cardFS interface {
	Rename(oldpath, newpath string) error
	Create(name string) (io.WriteCloser, error)
	MkdirAll(path string, perm os.FileMode) error
	Remove(name string) error
	RemoveAll(path string) error
	// SyncFile flushes a file made by Create to the card; call it before Close
	SyncFile(f io.WriteCloser) error
	// SyncDir flushes a folder's entries (new names, renames) to the card
	SyncDir(dir string) error
	Stat(name string) (os.FileInfo, error)
	ReadDir(name string) ([]os.DirEntry, error)
}

// cardfs is what every card write goes through.
var cardfs cardFS = realFS{}

type realFS struct{}

func (realFS) Rename(oldpath, newpath string) error {
	return withBusyRetry(func() error { return os.Rename(oldpath, newpath) })
}
func (realFS) Create(name string) (io.WriteCloser, error)   { return os.Create(name) }
func (realFS) MkdirAll(path string, perm os.FileMode) error { return os.MkdirAll(path, perm) }
func (realFS) Remove(name string) error {
	return withBusyRetry(func() error { return os.Remove(name) })
}
func (realFS) RemoveAll(path string) error {
	return withBusyRetry(func() error { return os.RemoveAll(path) })
}
func (realFS) Stat(name string) (os.FileInfo, error)      { return os.Stat(name) }
func (realFS) ReadDir(name string) ([]os.DirEntry, error) { return os.ReadDir(name) }

func (realFS) SyncFile(f io.WriteCloser) error {
	if s, ok := f.(interface{ Sync() error }); ok {
		return s.Sync()
	}
	return nil
}

func (realFS) SyncDir(dir string) error {
	if runtime.GOOS == "windows" {
		return nil // Windows cannot flush a folder handle; FAT32 entries are written with the file there
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	if err := d.Sync(); err != nil && runtime.GOOS != "darwin" {
		return err
	}
	return nil
}
