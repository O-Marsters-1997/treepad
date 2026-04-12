package workspace

import (
	"path/filepath"

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
	switch {
	case useCurrentFlag:
		return cwd, nil
	case sourcePath != "":
		return filepath.Abs(sourcePath)
	default:
		main, err := worktree.MainWorktree(worktrees)
		if err != nil {
			return "", err
		}
		return main.Path, nil
	}
}
