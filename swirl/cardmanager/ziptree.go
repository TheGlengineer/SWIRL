package main

// Copying and unpacking whole folders with their permissions and symbolic links intact. macOS apps
// (.app bundles) are folders, and the macOS updater and the Flycast unpacker rely on these.

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// extractZipTree unpacks a zip made with `ditto -c -k` or `zip -y` into dst, keeping modes and symlinks.
func extractZipTree(zr *zip.Reader, dst string) error {
	dst = filepath.Clean(dst)
	for _, f := range zr.File {
		name := filepath.FromSlash(f.Name)
		if strings.HasPrefix(name, "__MACOSX") || filepath.Base(name) == ".DS_Store" {
			continue
		}
		p := filepath.Join(dst, name)
		if p != dst && !strings.HasPrefix(p, dst+string(filepath.Separator)) {
			return fmt.Errorf("unsafe path in archive: %s", f.Name)
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case mode&os.ModeSymlink != 0:
			rc, err := f.Open()
			if err != nil {
				return err
			}
			target, err := io.ReadAll(io.LimitReader(rc, 4096))
			rc.Close()
			if err != nil {
				return err
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(p), string(target)))
			if filepath.IsAbs(string(target)) || (resolved != dst && !strings.HasPrefix(resolved, dst+string(filepath.Separator))) {
				return fmt.Errorf("unsafe link in archive: %s", f.Name)
			}
			os.MkdirAll(filepath.Dir(p), 0o755)
			os.Remove(p)
			if err := os.Symlink(string(target), p); err != nil {
				return err
			}
		default:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			rc, err := f.Open()
			if err != nil {
				return err
			}
			perm := mode.Perm()
			if perm == 0 {
				perm = 0o644
			}
			out, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, perm)
			if err != nil {
				rc.Close()
				return err
			}
			_, err = io.Copy(out, rc)
			rc.Close()
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			os.Chmod(p, perm)
		}
	}
	return nil
}

// copyTreeKeep copies a folder, keeping file modes and symbolic links (unlike copyTree, which follows them).
func copyTreeKeep(src, dst string) error {
	src = filepath.Clean(src)
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		out := filepath.Join(dst, rel)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			os.Remove(out)
			return os.Symlink(t, out)
		case info.IsDir():
			return os.MkdirAll(out, info.Mode().Perm()|0o700)
		case info.Mode().IsRegular():
			in, err := os.Open(p)
			if err != nil {
				return err
			}
			defer in.Close()
			o, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
			if err != nil {
				return err
			}
			if _, err := io.Copy(o, in); err != nil {
				o.Close()
				return err
			}
			return o.Close()
		}
		return errors.New("unsupported file " + p)
	})
}

// findApp returns the first .app bundle directly inside dir.
func findApp(dir string) string {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}
