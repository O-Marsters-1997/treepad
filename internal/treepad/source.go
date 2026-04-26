package treepad

import (
	"treepad/internal/treepad/repo"
	"treepad/internal/worktree"
)

// ResolveSourceDir is a pure function — no I/O.
// cwd is pre-fetched by the caller and used only when useCurrentFlag is true.
func ResolveSourceDir(
	useCurrentFlag bool,
	sourcePath string,
	cwd string,
	worktrees []worktree.Worktree,
) (string, error) {
	return repo.ResolveSourceDir(useCurrentFlag, sourcePath, cwd, worktrees)
}
