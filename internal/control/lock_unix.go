//go:build linux || darwin

package control

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// Holding the advisory lock prevents a second Web server or recovery command
// from invalidating the live server's logins and session records at startup.
func lockDatabase(path string) (*os.File, error) {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("database is in use; stop the Conduit server before recovery: %w", e)
	}
	return f, nil
}
