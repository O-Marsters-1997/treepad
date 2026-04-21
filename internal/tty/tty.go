//go:build !windows

// Package tty provides access to the process's controlling terminal.
package tty

import "os"

// Open opens the controlling terminal of the current process for reading and
// writing. Returns nil when no controlling terminal is available (CI, detached
// session, no-TTY environments).
func Open() *os.File {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil
	}
	return f
}
