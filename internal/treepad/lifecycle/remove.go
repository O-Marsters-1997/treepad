package lifecycle

import (
	"context"
	"fmt"

	"github.com/O-Marsters-1997/treepad/internal/treepad/cwd"
	"github.com/O-Marsters-1997/treepad/internal/treepad/deps"
	"github.com/O-Marsters-1997/treepad/internal/treepad/repo"
	"github.com/O-Marsters-1997/treepad/internal/worktree"
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

	found, err := worktree.FindOrErr(rc.Worktrees, in.Branch)
	if err != nil {
		return err
	}

	curDir, err := cwd.Resolve(in.Cwd)
	if err != nil {
		return err
	}
	if repo.CwdInside(curDir, found.Path) {
		return fmt.Errorf("cannot remove the worktree you are currently in; cd elsewhere first")
	}

	_, err = RemoveWorktreeAndArtifact(ctx, d, found, rc.Main, rc.OutputDir, in.Force)
	return err
}
