package treepad

import (
	"context"
	"fmt"

	"treepad/internal/artifact"
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
		openPath := res.WorktreePath
		if res.ArtifactPath != "" {
			openPath = res.ArtifactPath
		}
		d.Log.Step("opening...")
		openSpec := artifact.OpenSpec{Command: res.Cfg.Open.Command}
		openData := artifact.OpenData{
			ArtifactPath: openPath,
			Worktree:     artifact.ToWorktree(in.Branch, res.WorktreePath, res.RC.OutputDir),
		}
		if err := d.Opener.Open(ctx, openSpec, openData); err != nil {
			return fmt.Errorf("open: %w", err)
		}
	}
	if !in.Current {
		emitCD(d, res.WorktreePath)
	}
	return nil
}
