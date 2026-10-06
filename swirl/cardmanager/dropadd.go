package main

// Games dropped onto the window (2.17). The window runs in a browser, which never tells a page where a
// dropped file lives on disk, only its name and contents. So a drop streams each file to this process,
// which keeps them in a staging folder under the app's data folder, and then the normal Add games pipeline
// runs on that folder, exactly as if it had been picked with Browse. The staging folder goes when the job
// ends, however it ends. Add games from a folder on the PC stays the faster way for a multi gigabyte disc,
// since a drop copies it twice; the window says so.

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func stagingDir() string { return filepath.Join(appDataDir(), "dropped") }

var dropIDs sync.Map // id -> true while a drop is being received

// newDrop makes an empty staging folder and returns its id.
func newDrop() (string, error) {
	var b [8]byte
	rand.Read(b[:])
	id := hex.EncodeToString(b[:])
	if err := os.MkdirAll(filepath.Join(stagingDir(), id), 0o755); err != nil {
		return "", err
	}
	dropIDs.Store(id, true)
	return id, nil
}

// dropPath turns the relative path the browser gave (forward slashes, from the dropped item down) into a
// path inside the drop's folder, refusing anything that would leave it.
func dropPath(id, rel string) (string, error) {
	if _, ok := dropIDs.Load(id); !ok {
		return "", errors.New("this drop is no longer open")
	}
	rel = strings.ReplaceAll(rel, "\\", "/")
	parts := strings.Split(rel, "/")
	var clean []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || p == "." {
			continue
		}
		if p == ".." || strings.ContainsAny(p, ":*?\"<>|\x00") {
			return "", fmt.Errorf("bad file name %q", rel)
		}
		clean = append(clean, p)
	}
	if len(clean) == 0 {
		return "", errors.New("empty file name")
	}
	return filepath.Join(append([]string{stagingDir(), id}, clean...)...), nil
}

// receiveDrop stores one dropped file.
func receiveDrop(id, rel string, body io.Reader) (int64, error) {
	path, err := dropPath(id, rel)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, err
	}
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, body)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return n, err
	}
	return n, nil
}

// dropSources lists what was dropped: the top level entries of the staging folder, which is what the owner
// would have picked in Browse.
func dropSources(id string) ([]string, error) {
	dir := filepath.Join(stagingDir(), id)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		out = append(out, filepath.Join(dir, e.Name()))
	}
	if len(out) == 0 {
		return nil, errors.New("nothing arrived from the drop")
	}
	return out, nil
}

// discardDrop removes a drop's folder, used when the drop is abandoned or its job is over.
func discardDrop(id string) {
	if id == "" {
		return
	}
	dropIDs.Delete(id)
	if _, err := dropPath(id, "x"); err == nil || strings.Contains(err.Error(), "no longer open") {
		os.RemoveAll(filepath.Join(stagingDir(), id))
	}
}

// StartAddDropped runs Add games on a finished drop and cleans up after it.
func StartAddDropped(root, id, dats string) error {
	sources, err := dropSources(id)
	if err != nil {
		discardDrop(id)
		return err
	}
	if err := startAddGames(root, sources, dats, func() { discardDrop(id) }); err != nil {
		discardDrop(id)
		return err
	}
	return nil
}

// cleanDrops removes staging folders left by an earlier run (a crash, a closed window mid drop).
func cleanDrops() {
	entries, err := os.ReadDir(stagingDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if _, live := dropIDs.Load(e.Name()); !live {
			os.RemoveAll(filepath.Join(stagingDir(), e.Name()))
		}
	}
}
