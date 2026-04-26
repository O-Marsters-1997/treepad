package lifecycle

import (
	"context"

	"treepad/internal/config"
	"treepad/internal/treepad/deps"
)

type createWorktreeResult = CreateResult

type syncTarget = SyncTarget

func createWorktreeWithSync(ctx context.Context, d deps.Deps, branch, base, outputDir string) (createWorktreeResult, error) {
	return CreateWorktreeWithSync(ctx, d, branch, base, outputDir)
}

func openWorktree(ctx context.Context, d deps.Deps, openCmd []string, branch, wtPath, artifactPath, outputDir string) error {
	return OpenWorktree(ctx, d, openCmd, branch, wtPath, artifactPath, outputDir)
}

func loadAndSync(
	ctx context.Context, d deps.Deps, sourceDir string,
	extraPatterns []string, targets []syncTarget,
	repoSlug, outputDir string,
) (config.Config, error) {
	return LoadAndSync(ctx, d, sourceDir, extraPatterns, targets, repoSlug, outputDir)
}
