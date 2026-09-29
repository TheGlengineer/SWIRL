package main

// One writer per card. Every handler and job that writes to a card holds the card's lock for its whole
// run, so an install, a remove and an edit never interleave. Reading (scan, covers) needs no lock.

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var errCardBusy = errors.New("Another operation is running on this card. Wait for it to finish.")

type cardLockState struct {
	mu sync.Mutex
	op string // the operation that holds the lock ("" when free); guarded by cardLocksMu
}

var (
	cardLocksMu sync.Mutex
	cardLocks   = map[string]*cardLockState{}
	// cardLockWait is how long a second writer waits for a short operation to finish before giving up
	cardLockWait = 2 * time.Second
)

// cardKey names a card the same way whichever spelling of its path a request uses.
func cardKey(root string) string {
	r := strings.TrimSpace(root)
	if a, err := filepath.Abs(r); err == nil {
		r = a
	}
	r = filepath.Clean(r)
	if runtime.GOOS != "linux" { // Windows and macOS paths are not case sensitive
		r = strings.ToLower(r)
	}
	return r
}

func cardLockFor(root string) *cardLockState {
	k := cardKey(root)
	cardLocksMu.Lock()
	defer cardLocksMu.Unlock()
	l := cardLocks[k]
	if l == nil {
		l = &cardLockState{}
		cardLocks[k] = l
	}
	return l
}

// lockCard takes the card's lock for op and returns the function that releases it. A card that stays
// busy for longer than cardLockWait gives errCardBusy.
func lockCard(root, op string) (func(), error) {
	l := cardLockFor(root)
	deadline := time.Now().Add(cardLockWait)
	for !l.mu.TryLock() {
		if time.Now().After(deadline) {
			return nil, errCardBusy
		}
		time.Sleep(20 * time.Millisecond)
	}
	cardLocksMu.Lock()
	l.op = op
	cardLocksMu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			cardLocksMu.Lock()
			l.op = ""
			cardLocksMu.Unlock()
			l.mu.Unlock()
		})
	}, nil
}

// anyCardBusy is true while any card is being written.
func anyCardBusy() bool {
	cardLocksMu.Lock()
	defer cardLocksMu.Unlock()
	for _, l := range cardLocks {
		if l.op != "" {
			return true
		}
	}
	return false
}

// uniqueBackupPath returns SWIRL_BACKUP/<prefix><date_time>, with _2, _3 ... added when that folder
// already exists (two operations in the same second).
func uniqueBackupPath(root, prefix string) string {
	base := filepath.Join(root, backupDir, prefix+time.Now().Format("20060102_150405"))
	p := base
	for i := 2; fileExists(p); i++ {
		p = fmt.Sprintf("%s_%d", base, i)
	}
	return p
}
