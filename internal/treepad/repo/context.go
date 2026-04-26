package repo

import (
	"context"

	"treepad/internal/worktree"
)

func loadRepoContext(
	ctx context.Context,
	runner worktree.CommandRunner,
	explicitOutputDir string) (Context, error) {
	return Load(ctx, runner, explicitOutputDir)
}

func listWorktrees(ctx context.Context, runner worktree.CommandRunner) ([]worktree.Worktree, error) {
	return ListWorktrees(ctx, runner)
}

func resolveOutputDir(explicit, repoSlug string) (string, error) {
	return ResolveOutputDir(explicit, repoSlug)
}
