package treepad

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"treepad/internal/artifact"
	"treepad/internal/slug"
)

type NewInput struct {
	Branch    string
	Base      string
	Open      bool
	Current   bool
	OutputDir string
}

func New(ctx context.Context, d Deps, in NewInput) error {
	rc, err := loadRepoContext(ctx, d, in.OutputDir)
	if err != nil {
		return err
	}

	worktreePath := filepath.Join(filepath.Dir(rc.Main.Path), rc.Slug+"-"+slug.Slug(in.Branch))
	slog.Debug("derived worktree path", "path", worktreePath)

	if _, err := d.Runner.Run(ctx, "git", "worktree", "add", "-b", in.Branch, worktreePath, in.Base); err != nil {
		return fmt.Errorf("git worktree add: %w", err)
	}
	d.Log.OK("created worktree at %s", worktreePath)

	cfg, err := loadAndSync(d, rc.Main.Path, nil, []syncTarget{{path: worktreePath, branch: in.Branch}})
	if err != nil {
		return err
	}

	data := templateData(rc.Slug, in.Branch, worktreePath, rc.OutputDir)
	artifactPath, err := artifact.Write(artifactSpec(cfg.Artifact), rc.OutputDir, data)
	if err != nil {
		return fmt.Errorf("write artifact: %w", err)
	}
	slog.Debug("wrote artifact", "outputDir", rc.OutputDir, "branch", in.Branch)

	if in.Open {
		openPath := worktreePath
		if artifactPath != "" {
			openPath = artifactPath
		}
		d.Log.Step("opening...")
		openSpec := artifact.OpenSpec{Command: cfg.Open.Command}
		openData := artifact.OpenData{
			ArtifactPath: openPath,
			Worktree:     artifact.ToWorktree(in.Branch, worktreePath, rc.OutputDir),
		}
		if err := d.Opener.Open(ctx, openSpec, openData); err != nil {
			return fmt.Errorf("open: %w", err)
		}
	}
	if !in.Current {
		emitCD(d, worktreePath)
	}
	return nil
}
