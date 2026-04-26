package treepad

import "treepad/internal/treepad/repo"

func cwdInside(cwd, wtPath string) bool {
	return repo.CwdInside(cwd, wtPath)
}

func requireCwdInside(cwd, wtPath, msg string) error {
	return repo.RequireCwdInside(cwd, wtPath, msg)
}
