//go:build !windows

// Package tty provides access to the process's controlling terminal.
package tty

import (
	"os"
	"syscall"
)

// Open opens the controlling terminal of the current process for reading and
// writing. Returns nil when no controlling terminal is available (CI, detached
// session, no-TTY environments).
//
// Used as a fallback when parent stdio is not a terminal (e.g. old-wrapper
// invocation where stdout is captured by a subshell). Note: Bun-compiled
// agents on macOS reject /dev/tty-opened fds for TTY stream construction;
// callers should prefer inheriting parent stdio directly when it is already a
// terminal.
func Open() *os.File {
	fd, err := syscall.Open("/dev/tty", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil
	}
	return os.NewFile(uintptr(fd), "/dev/tty")
}
