package treepad

import (
	"context"
	"fmt"

	"treepad/internal/worktree"
)

type SwitchInput struct {
	Branch string
}

func (s *Service) Switch(ctx context.Context, in SwitchInput) error {
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
