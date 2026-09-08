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
	// Force wipes a dirty worktree and deletes an unmerged branch. It never
	// checks the branch is safe to delete — the caller is asserting that.
	Force bool
	// Merged asserts the branch has already merged upstream, skipping only
	// the local merge-base check that would otherwise refuse it. It still
	// refuses a dirty worktree or unpushed commits.
	Merged bool
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
	if in.Force && in.Merged {
		return fmt.Errorf("--force and --merged are mutually exclusive")
	}

	switch {
	case in.Merged:
		dirty, err := worktree.Dirty(ctx, d.Runner, found.Path)
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("worktree has uncommitted changes: %s", found.Path)
		}
		ahead, _, hasUpstream, err := worktree.AheadBehind(ctx, d.Runner, found.Path)
		if err != nil {
			return err
		}
		if hasUpstream && ahead > 0 {
			return fmt.Errorf("branch %q has %d unpushed commit(s)", in.Branch, ahead)
		}
	case !in.Force:
		if _, err := d.Runner.Run(ctx, "git", "merge-base", "--is-ancestor", in.Branch, rc.Main.Branch); err != nil {
			return fmt.Errorf(
				"branch %q is not merged into %s; pass --merged if it merged upstream, or --force to delete it anyway",
				in.Branch, rc.Main.Branch)
		}
	}

	mode := RemoveMode{WipeDirty: in.Force, DeleteUnmerged: in.Force || in.Merged}
	_, err = RemoveWorktreeAndArtifact(ctx, d, found, rc.Main, rc.OutputDir, mode)
	return err
}
