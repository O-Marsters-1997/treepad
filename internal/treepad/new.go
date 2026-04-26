package treepad

import (
	"context"
	"fmt"
)

type NewInput struct {
	Branch    string
	Base      string
	Open      bool
	Current   bool
	OutputDir string
}

func New(ctx context.Context, d Deps, in NewInput) error {
	res, err := createWorktreeWithSync(ctx, d, in.Branch, in.Base, in.OutputDir)
	if err != nil {
		return err
	}

	if in.Open {
		d.Log.Step("opening...")
		if err := openWorktree(ctx, d, res.Cfg.Open.Command,
			in.Branch, res.WorktreePath, res.ArtifactPath, res.RC.OutputDir); err != nil {
			return fmt.Errorf("open: %w", err)
		}
	}
	if !in.Current {
		emitCD(d, res.WorktreePath)
	}
	return nil
}
