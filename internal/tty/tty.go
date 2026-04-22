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
// syscall.Open + os.NewFile is used instead of os.OpenFile so that Go's
// runtime poller does not flip O_NONBLOCK on the file description. Node/Bun
// children reject a non-blocking TTY fd when setting up their stdio streams.
func Open() *os.File {
	fd, err := syscall.Open("/dev/tty", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil
	}
	return os.NewFile(uintptr(fd), "/dev/tty")
}
