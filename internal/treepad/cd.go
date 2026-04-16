package treepad

import (
	"context"
	"fmt"

	"treepad/internal/worktree"
)

type CDInput struct {
	Branch string
}

func CD(ctx context.Context, d Deps, in CDInput) error {
	worktrees, err := listWorktrees(ctx, d)
	if err != nil {
		return err
	}

	wt, ok := worktree.FindByBranch(worktrees, in.Branch)
	if !ok {
		return fmt.Errorf("no worktree found for branch %q; create one with: treepad new %s", in.Branch, in.Branch)
	}

	emitCD(d, wt.Path)
	return nil
}
