//go:build windows

// Package tty provides access to the process's controlling terminal.
package tty

import "os"

// Open returns nil on Windows; controlling terminal access is not supported.
func Open() *os.File { return nil }
