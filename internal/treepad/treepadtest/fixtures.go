package treepadtest

import "fmt"

// Porcelain fixture builders

// TwoWorktreePorcelain produces two worktrees; IsMain is false for both since
// the paths don't exist on disk.
var TwoWorktreePorcelain = []byte(`worktree /repo/main
HEAD abc123
branch refs/heads/main

worktree /repo/feat
HEAD def456
branch refs/heads/feat

`)

// ThreeWorktreePorcelain produces three worktrees.
var ThreeWorktreePorcelain = []byte(`worktree /repo/main
HEAD abc123
branch refs/heads/main

worktree /repo/feat
HEAD def456
branch refs/heads/feat

worktree /repo/other
HEAD ghi789
branch refs/heads/other

`)

// MainWorktreePorcelain builds porcelain output where mainPath has a real .git dir.
func MainWorktreePorcelain(mainPath string) []byte {
	return fmt.Appendf(nil, "worktree %s\nHEAD abc123\nbranch refs/heads/main\n\n", mainPath)
}

// TwoWorktreePorcelainWithMain builds porcelain for a main + feat worktree pair.
func TwoWorktreePorcelainWithMain(mainPath, featPath string) []byte {
	return fmt.Appendf(nil,
		"worktree %s\nHEAD abc123\nbranch refs/heads/main\n\nworktree %s\nHEAD def456\nbranch refs/heads/feat\n\n",
		mainPath, featPath,
	)
}

// TwoWorktreePorcelainWithPrunable builds porcelain where the second worktree is prunable.
func TwoWorktreePorcelainWithPrunable(mainPath, prunablePath string) []byte {
	return fmt.Appendf(nil,
		"worktree %s\nHEAD abc123\nbranch refs/heads/main\n\n"+
			"worktree %s\nHEAD def456\nbranch refs/heads/stale-branch\n"+
			"prunable gitdir file points to non-existent location\n\n",
		mainPath, prunablePath,
	)
}

// WorktreePorcelainWithPath builds porcelain output with a controllable path.
func WorktreePorcelainWithPath(branch, path string) []byte {
	return fmt.Appendf(nil, "worktree %s\nHEAD abc123\nbranch refs/heads/%s\n\n", path, branch)
}

// ThreeWorktreePorcelainWithMain builds porcelain for three worktrees.
func ThreeWorktreePorcelainWithMain(mainPath, feat1Path, feat2Path string) []byte {
	return fmt.Appendf(nil,
		"worktree %s\nHEAD abc123\nbranch refs/heads/main\n\n"+
			"worktree %s\nHEAD def456\nbranch refs/heads/feat\n\n"+
			"worktree %s\nHEAD ghi789\nbranch refs/heads/other\n\n",
		mainPath, feat1Path, feat2Path,
	)
}
