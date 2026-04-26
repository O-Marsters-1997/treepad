package lifecycle

import (
	"context"
	"fmt"
	"treepad/internal/treepad/deps"
)

// NewInput parameterises a tp new invocation.
type NewInput struct {
	Branch    string
	Base      string
	Open      bool
	Current   bool
	OutputDir string
}

// New creates a new worktree and returns the path to cd into (or "" when Current is true).
func New(ctx context.Context, d deps.Deps, in NewInput) (string, error) {
	res, err := CreateWorktreeWithSync(ctx, d, in.Branch, in.Base, in.OutputDir)
	if err != nil {
		return "", err
	}

	if in.Open {
		d.Log.Step("opening...")
		if err := OpenWorktree(ctx, d, res.Cfg.Open.Command,
			in.Branch, res.WorktreePath, res.ArtifactPath, res.RC.OutputDir); err != nil {
			return "", fmt.Errorf("open: %w", err)
		}
	}
	if in.Current {
		return "", nil
	}
	return res.WorktreePath, nil
}
