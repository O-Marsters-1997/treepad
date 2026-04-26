package treepad

import "treepad/internal/treepad/repo"

func cwdInside(cwd, wtPath string) bool {
	return repo.CwdInside(cwd, wtPath)
}
