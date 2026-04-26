package treepad

import (
	"context"
	"errors"
	"fmt"
	"os"

	"treepad/internal/worktree"
)

type RemoveInput struct {
	Branch    string
	OutputDir string
	Force     bool
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
}

func Remove(ctx context.Context, d Deps, in RemoveInput) error {
	rc, err := loadRepoContext(ctx, d, in.OutputDir)
	if err != nil {
		return err
	}

	if in.Branch == rc.Main.Branch {
		return errors.New("cannot remove the main worktree")
	}

	found, ok := worktree.FindByBranch(rc.Worktrees, in.Branch)
	if !ok {
		return fmt.Errorf("no worktree found for branch %q", in.Branch)
	}

	cwd := in.Cwd
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("get current directory: %w", err)
		}
	}
	if cwdInside(cwd, found.Path) {
		return errors.New("cannot remove the worktree you are currently in; cd elsewhere first")
	}

	return removeWorktreeAndArtifact(ctx, d, found, rc.Main, rc.OutputDir, in.Force)
}
