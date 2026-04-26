package lifecycle

import (
	"context"
	"fmt"
	"os"

	"treepad/internal/treepad/deps"
	"treepad/internal/treepad/repo"
	"treepad/internal/worktree"
)

// RemoveInput parameterises a tp remove invocation.
type RemoveInput struct {
	Branch    string
	OutputDir string
	Force     bool
	// Cwd overrides os.Getwd for testing the cwd-inside guard.
	Cwd string
}

// Remove removes a worktree and its artifact.
func Remove(ctx context.Context, d deps.Deps, in RemoveInput) error {
	rc, err := repo.Load(ctx, d.Runner, in.OutputDir)
	if err != nil {
		return err
	}

	if in.Branch == rc.Main.Branch {
		return fmt.Errorf("cannot remove the main worktree")
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
	if repo.CwdInside(cwd, found.Path) {
		return fmt.Errorf("cannot remove the worktree you are currently in; cd elsewhere first")
	}

	return RemoveWorktreeAndArtifact(ctx, d, found, rc.Main, rc.OutputDir, in.Force)
}
