package main

// On Windows a file that was just written is often held open for a moment by something else: the
// antivirus scanning it, the search indexer, Explorer making a thumbnail. A rename or delete then fails
// with "Access is denied" or "being used by another process" although nothing is wrong with the card.
// Every rename and delete on the card is tried again for a few seconds before it is reported, so an
// Update SWIRL does not stop half way because Defender looked at the new menu disc. Other systems do not
// give these errors for an open file, so they get one attempt.

import (
	"runtime"
	"time"
)

const busyRetryFor = 4 * time.Second

func withBusyRetry(op func() error) error {
	err := op()
	if err == nil || runtime.GOOS != "windows" || !isBusyError(err) {
		return err
	}
	deadline := time.Now().Add(busyRetryFor)
	wait := 50 * time.Millisecond
	for time.Now().Before(deadline) {
		time.Sleep(wait)
		if wait < 500*time.Millisecond {
			wait *= 2
		}
		if err = op(); err == nil || !isBusyError(err) {
			return err
		}
	}
	return err
}
