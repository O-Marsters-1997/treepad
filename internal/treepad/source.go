package treepad

import (
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
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
