//go:build darwin

package sync

import (
	"os"

	"golang.org/x/sys/unix"
)

func cloneFile(src, dst string) error {
	if err := unix.Clonefile(src, dst, 0); err == nil {
		return nil
	}
	// Retry once after removing a pre-existing destination.
	_ = os.Remove(dst)
	if unix.Clonefile(src, dst, 0) == nil {
		return nil
	}
	return errCloneUnsupported
}

func cloneTree(src, dst string) error {
	if err := unix.Clonefile(src, dst, 0); err == nil {
		return nil
	}
	_ = os.RemoveAll(dst)
	if unix.Clonefile(src, dst, 0) == nil {
		return nil
	}
	return errCloneUnsupported
}
