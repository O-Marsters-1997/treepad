package treepad

import (
	"context"
	"fmt"

	"treepad/internal/worktree"
)

type CDInput struct {
	Branch string
}

func (s *Service) CD(ctx context.Context, in CDInput) error {
	worktrees, err := s.listWorktrees(ctx)
	if err != nil {
		return err
	}

	wt, ok := worktree.FindByBranch(worktrees, in.Branch)
	if !ok {
		return fmt.Errorf("no worktree found for branch %q; create one with: treepad new %s", in.Branch, in.Branch)
	}

	s.emitCD(wt.Path)
	return nil
}
