package treepad

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"treepad/internal/slug"
	"treepad/internal/worktree"
)

// repoContext captures the values derived from listing worktrees at the
// beginning of every treepad operation: the worktrees themselves, the main
// one, a slug derived from its path, and the artifact output directory.
type repoContext struct {
	Worktrees []worktree.Worktree
	Main      worktree.Worktree
	Slug      string
	OutputDir string
}

// loadRepoContext runs the prologue shared by every public operation:
// list worktrees → find main → derive slug → resolve output directory.
// An empty explicitOutputDir means "derive the default".
func loadRepoContext(ctx context.Context, d Deps, explicitOutputDir string) (repoContext, error) {
	worktrees, err := listWorktrees(ctx, d)
	if err != nil {
		return repoContext{}, err
	}
	main, err := worktree.MainWorktree(worktrees)
	if err != nil {
		return repoContext{}, err
	}
	repoSlug := slug.Slug(filepath.Base(main.Path))
	outputDir, err := resolveOutputDir(explicitOutputDir, repoSlug)
	if err != nil {
		return repoContext{}, err
	}
	return repoContext{
		Worktrees: worktrees,
		Main:      main,
		Slug:      repoSlug,
		OutputDir: outputDir,
	}, nil
}

func listWorktrees(ctx context.Context, d Deps) ([]worktree.Worktree, error) {
	worktrees, err := worktree.List(ctx, d.Runner)
	if err != nil {
		return nil, fmt.Errorf("list worktrees: %w", err)
	}
	if len(worktrees) == 0 {
		return nil, fmt.Errorf("no git worktrees found")
	}
	slog.Debug("discovered worktrees", "count", len(worktrees))
	return worktrees, nil
}

func resolveOutputDir(explicit string, repoSlug string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	return filepath.Join(home, repoSlug+"-workspaces"), nil
}
