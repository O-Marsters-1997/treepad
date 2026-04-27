//go:build !darwin && !linux

package sync

func cloneFile(_, _ string) error { return errCloneUnsupported }
func cloneTree(_, _ string) error { return errCloneUnsupported }
