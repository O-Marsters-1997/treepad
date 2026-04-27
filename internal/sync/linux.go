//go:build linux

package sync

import (
	"os"

	"golang.org/x/sys/unix"
)

func cloneFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return errCloneUnsupported
	}
	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return errCloneUnsupported
	}
	defer func() { _ = out.Close() }()

	for {
		n, err := unix.CopyFileRange(int(in.Fd()), nil, int(out.Fd()), nil, 1<<30, 0)
		if err != nil {
			return errCloneUnsupported
		}
		if n == 0 {
			return out.Close()
		}
	}
}

func cloneTree(_, _ string) error {
	return errCloneUnsupported
}
