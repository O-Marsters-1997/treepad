package lifecycle

import (
	"context"

	"github.com/O-Marsters-1997/treepad/internal/profile"
	"github.com/O-Marsters-1997/treepad/internal/treepad/cwd"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
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
	p := profile.OrDisabled(d.Profiler)

	repoLoadDone := p.Stage("repo.load")
	rc, err := repo.Load(ctx, d.Runner, in.OutputDir)
	repoLoadDone()
	if err != nil {
		return err
	}

	curDir, err := cwd.Resolve(in.Cwd)
	if err != nil {
		return err
	}

	gatherDone := p.Stage("gather")
	var sel pruneSelection
	if in.All {
		sel, err = gatherAll(rc, curDir)
	} else {
		sel, err = gatherMerged(ctx, d, rc, curDir, in.Base)
	}
	gatherDone()
	if err != nil {
		return err
	}

	return executePrune(ctx, d, rc, sel, in.DryRun, in.Yes)
}
