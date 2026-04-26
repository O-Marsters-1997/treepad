package lifecycle

import (
	"context"
	"fmt"
	"os"
	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/repo"
	"treepad/internal/worktree"
)

type PruneInput struct {
	Base      string
	OutputDir string
	DryRun    bool
	All       bool
	Yes       bool
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
}

type pruneSelection struct {
	candidates []worktree.Worktree
	force      bool
	verb       string
	emptyMsg   string
}

// Prune removes merged (or all non-main) worktrees.
func Prune(ctx context.Context, d deps.Deps, in PruneInput) error {
	rc, err := repo.Load(ctx, d.Runner, in.OutputDir)
	if err != nil {
		return err
	}

	cwd := in.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
	}

	var sel pruneSelection
	if in.All {
		sel, err = gatherAll(rc, cwd)
	} else {
		sel, err = gatherMerged(ctx, d, rc, cwd, in.Base)
	}
	if err != nil {
		return err
	}

	return executePrune(ctx, d, rc, sel, in.DryRun, in.Yes)
}
