package treepad

import "github.com/O-Marsters-1997/treepad/internal/treepad/repo"

func cwdInside(cwd, wtPath string) bool {
	return repo.CwdInside(cwd, wtPath)
}
